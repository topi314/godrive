package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database/dbq"
	"golang.org/x/oauth2"
)

const RefreshCookieName = "X-Refresh-Token"

type sessionExpiryKey struct{}

func GetSessionExpiry(r *http.Request) time.Time {
	v, _ := r.Context().Value(sessionExpiryKey{}).(time.Time)
	return v
}

func (s *Server) refreshDeadline(sess dbq.Session) time.Time {
	d := config.DefaultRefreshTTL
	if s.cfg.AuthEnabled() {
		d = s.cfg.Auth.RefreshTokenLifespan.Duration
	}
	return sess.CreatedAt.Add(d)
}

func (s *Server) setAuthCookies(w http.ResponseWriter, sessionID string, sessionExp, refreshExp time.Time) {
	secure := s.cfg.AuthEnabled() && s.cfg.Auth.Secure
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  sessionExp,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  refreshExp,
	})
}

func (s *Server) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: RefreshCookieName, Value: "", Path: "/", MaxAge: -1})
}

func skipSessionRefresh(r *http.Request) bool {
	p := r.URL.Path
	return p == "/api/login" || p == "/api/logout" || p == "/api/callback" || p == "/api/refresh"
}

func (s *Server) tryRefresh(w http.ResponseWriter, r *http.Request) (*UserInfo, time.Time) {
	if s.auth == nil || skipSessionRefresh(r) {
		return nil, time.Time{}
	}
	c, err := r.Cookie(RefreshCookieName)
	if err != nil || c.Value == "" {
		c, err = r.Cookie(SessionCookieName)
	}
	if err != nil || c.Value == "" {
		return nil, time.Time{}
	}
	info, exp, err := s.rotateSession(r.Context(), w, c.Value)
	if err != nil {
		return nil, time.Time{}
	}
	return info, exp
}

func (s *Server) rotateSession(ctx context.Context, w http.ResponseWriter, sessionID string) (*UserInfo, time.Time, error) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	sess, err := s.store.Q.GetSession(ctx, sessionID)
	if err != nil {
		return nil, time.Time{}, err
	}
	now := time.Now().UTC()
	if now.After(s.refreshDeadline(sess)) {
		_ = s.store.Q.DeleteSession(ctx, sessionID)
		s.clearAuthCookies(w)
		return nil, time.Time{}, errors.New("refresh expired")
	}

	access, refresh, rawID := sess.AccessToken, sess.RefreshToken, sess.IDToken
	if s.auth != nil {
		active := false
		if sess.AccessToken != "" {
			var err error
			active, err = s.auth.Introspect(ctx, sess.AccessToken, "access_token")
			if err != nil {
				slog.Warn("oidc introspect failed", slog.Any("err", err))
				active = false
			}
		}
		if !active {
			if sess.RefreshToken == "" {
				_ = s.store.Q.DeleteSession(ctx, sessionID)
				s.clearAuthCookies(w)
				return nil, time.Time{}, errors.New("oidc token inactive")
			}
			tok, err := s.auth.Config.TokenSource(ctx, &oauth2.Token{
				AccessToken:  sess.AccessToken,
				RefreshToken: sess.RefreshToken,
				Expiry:       now.Add(-time.Minute),
			}).Token()
			if err != nil {
				slog.Warn("oidc token refresh failed", slog.Any("err", err))
				_ = s.store.Q.DeleteSession(ctx, sessionID)
				s.clearAuthCookies(w)
				return nil, time.Time{}, err
			}
			access = tok.AccessToken
			if tok.RefreshToken != "" {
				refresh = tok.RefreshToken
			}
			if idt, ok := tok.Extra("id_token").(string); ok && idt != "" {
				rawID = idt
			}
			if err := s.syncUserFromOIDC(ctx, sess.UserID, tok, rawID); err != nil {
				slog.Warn("sync user from oidc failed", slog.Any("err", err))
			}
		} else if err := s.syncUserFromOIDC(ctx, sess.UserID, &oauth2.Token{
			AccessToken:  access,
			RefreshToken: refresh,
		}, rawID); err != nil {
			slog.Warn("sync user from oidc failed", slog.Any("err", err))
		}
	}

	user, err := s.store.Q.GetUser(ctx, sess.UserID)
	if err != nil {
		return nil, time.Time{}, err
	}
	info := s.userToInfo(user)
	if !s.hasAccess(info) {
		_ = s.store.Q.DeleteSession(ctx, sessionID)
		s.clearAuthCookies(w)
		return nil, time.Time{}, errors.New("access denied")
	}

	newID := randomID(32)
	sessionTTL := config.DefaultSessionTTL
	if s.cfg.AuthEnabled() {
		sessionTTL = s.cfg.Auth.SessionLifespan.Duration
	}
	sessionExp := now.Add(sessionTTL)
	refreshExp := s.refreshDeadline(sess)
	_, err = s.store.Q.UpsertSession(ctx, dbq.UpsertSessionParams{
		ID:           newID,
		UserID:       sess.UserID,
		AccessToken:  access,
		Expiry:       sessionExp,
		RefreshToken: refresh,
		IDToken:      rawID,
		CreatedAt:    sess.CreatedAt,
		UpdatedAt:    now,
	})
	if err != nil {
		return nil, time.Time{}, err
	}
	if newID != sessionID {
		_ = s.store.Q.DeleteSession(ctx, sessionID)
	}
	s.setAuthCookies(w, newID, sessionExp, refreshExp)
	return info, sessionExp, nil
}

func (s *Server) syncUserFromOIDC(ctx context.Context, userID string, tok *oauth2.Token, rawIDToken string) error {
	if s.auth == nil || tok == nil {
		return nil
	}
	var claims oidcClaims
	if rawIDToken != "" {
		if idToken, err := s.auth.Verifier.Verify(ctx, rawIDToken); err == nil {
			_ = idToken.Claims(&claims)
		}
	}
	if ui, err := s.auth.Provider.UserInfo(ctx, oauth2.StaticTokenSource(tok)); err == nil {
		var fromUI oidcClaims
		if err := ui.Claims(&fromUI); err == nil {
			claims.merge(fromUI)
		}
	}
	groups := s.cfg.Auth.Groups.MapOIDCGroups(claims.Groups.Strings())
	groupsJSON, _ := json.Marshal(groups)
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = userID
	}
	now := time.Now().UTC()
	_, err := s.store.Q.UpsertUser(ctx, dbq.UpsertUserParams{
		ID:        userID,
		Username:  username,
		Email:     claims.Email,
		Home:      "/",
		Groups:    string(groupsJSON),
		CreatedAt: now,
		UpdatedAt: now,
	})
	return err
}

func (s *Server) Refresh(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.AuthEnabled() || s.auth == nil {
		s.writeError(w, r, errors.New("auth not configured"), http.StatusNotImplemented)
		return
	}
	c, err := r.Cookie(RefreshCookieName)
	if err != nil || c.Value == "" {
		c, err = r.Cookie(SessionCookieName)
	}
	if err != nil || c.Value == "" {
		s.writeError(w, r, errors.New("unauthorized"), http.StatusUnauthorized)
		return
	}
	info, exp, err := s.rotateSession(r.Context(), w, c.Value)
	if err != nil || info == nil {
		s.writeError(w, r, errors.New("unauthorized"), http.StatusUnauthorized)
		return
	}
	s.writeJSON(w, map[string]any{
		"ok":                 true,
		"session_expires_at": exp.UTC().Format(time.RFC3339),
	}, http.StatusOK)
}
