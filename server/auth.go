package server

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database/dbq"
	"github.com/topi314/godrive/server/oidc"
	"golang.org/x/oauth2"
)

// oidcClaims holds profile fields from an ID token and/or UserInfo response.
type oidcClaims struct {
	Email             string     `json:"email"`
	PreferredUsername string     `json:"preferred_username"`
	Groups            stringList `json:"groups"`
}

func (c *oidcClaims) merge(o oidcClaims) {
	if c.Email == "" {
		c.Email = o.Email
	}
	if c.PreferredUsername == "" {
		c.PreferredUsername = o.PreferredUsername
	}
	if len(o.Groups) > 0 {
		c.Groups = o.Groups
	}
}

// stringList accepts JSON string or []string (IdP quirks).
type stringList []string

func (s *stringList) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = nil
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*s = arr
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	if one == "" {
		*s = nil
		return nil
	}
	*s = []string{one}
	return nil
}

func (s stringList) Strings() []string { return []string(s) }

const SessionCookieName = "X-Session-ID"

type authKey struct{}

var UserInfoKey = authKey{}

type Session struct {
	AccessToken  string
	Expiry       time.Time
	RefreshToken string
	IDToken      string
	UserID       string
}

type UserInfo struct {
	Subject  string   `json:"sub"`
	Email    string   `json:"email"`
	Home     string   `json:"home"`
	Groups   []string `json:"groups"`
	Username string   `json:"preferred_username"`
}

func GetUserInfo(r *http.Request) *UserInfo {
	v, _ := r.Context().Value(UserInfoKey).(*UserInfo)
	return v
}

func (s *Server) isAdmin(info *UserInfo) bool {
	if !s.cfg.AuthEnabled() {
		return true
	}
	admin := s.cfg.Auth.Groups.AdminGroup()
	return info != nil && admin != "" && containsString(info.Groups, admin)
}

func (s *Server) isAccess(info *UserInfo) bool {
	if info == nil || !s.cfg.AuthEnabled() {
		return false
	}
	access := s.cfg.Auth.Groups.AccessGroup()
	return access != "" && containsString(info.Groups, access)
}

func (s *Server) isGuest(info *UserInfo) bool {
	return userIsGuest(info)
}

func (s *Server) hasAccess(info *UserInfo) bool {
	if !s.cfg.AuthEnabled() {
		return true
	}
	if !s.cfg.Auth.Groups.Guest && s.isGuest(info) {
		return false
	}
	return s.isAdmin(info) || s.isAccess(info) || s.isGuest(info)
}

func parseGroupsJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var groups []string
	if err := json.Unmarshal([]byte(raw), &groups); err == nil {
		return groups
	}
	return []string{raw}
}

func (s *Server) mappedGroups(groups []string) []string {
	if !s.cfg.AuthEnabled() {
		return nil
	}
	return s.cfg.Auth.Groups.MapOIDCGroups(groups)
}

func (s *Server) availableGroups() []string {
	if !s.cfg.AuthEnabled() {
		return nil
	}
	return s.cfg.Auth.Groups.AvailableGroups()
}

func (s *Server) userPublicJSON(ctx context.Context, u dbq.User) map[string]any {
	_ = ctx
	info := s.userToInfo(u)
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"avatar":     gravatarURL(u.Email),
		"home":       u.Home,
		"groups":     info.Groups,
		"is_admin":   s.isAdmin(info),
		"is_access":  s.isAccess(info),
		"created_at": u.CreatedAt,
		"updated_at": u.UpdatedAt,
	}
}

func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info, sessExp := s.resolveUser(r)
		if info == nil && !s.cfg.AuthEnabled() {
			// Open mode: no OIDC — every request is a local admin.
			info = &UserInfo{Subject: "local", Username: "local", Groups: []string{"admin"}, Home: "/"}
		}
		if info == nil && s.cfg.AuthEnabled() {
			info, sessExp = s.tryRefresh(w, r)
		}
		if info != nil {
			ctx := context.WithValue(r.Context(), UserInfoKey, info)
			if !sessExp.IsZero() {
				ctx = context.WithValue(ctx, sessionExpiryKey{}, sessExp)
			}
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) resolveUser(r *http.Request) (*UserInfo, time.Time) {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		token := strings.TrimSpace(auth[7:])
		hash := hashToken(token)
		row, err := s.store.Q.GetAPITokenByHash(r.Context(), hash)
		if err == nil {
			user, err := s.store.Q.GetUser(r.Context(), row.UserID)
			if err == nil {
				return s.userToInfo(user), time.Time{}
			}
		}
	}

	c, err := r.Cookie(SessionCookieName)
	if err != nil || c.Value == "" {
		return nil, time.Time{}
	}
	sess, err := s.store.Q.GetSession(r.Context(), c.Value)
	if err != nil {
		return nil, time.Time{}
	}
	now := time.Now()
	if now.After(s.refreshDeadline(sess)) {
		_ = s.store.Q.DeleteSession(r.Context(), c.Value)
		return nil, time.Time{}
	}
	if now.After(sess.Expiry) {
		return nil, time.Time{}
	}
	user, err := s.store.Q.GetUser(r.Context(), sess.UserID)
	if err != nil {
		return nil, time.Time{}
	}
	return s.userToInfo(user), sess.Expiry
}

func (s *Server) userToInfo(u dbq.User) *UserInfo {
	return &UserInfo{
		Subject:  u.ID,
		Email:    u.Email,
		Home:     u.Home,
		Groups:   s.mappedGroups(parseGroupsJSON(u.Groups)),
		Username: u.Username,
	}
}

func (s *Server) RequireAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.AuthEnabled() {
			next.ServeHTTP(w, r)
			return
		}
		info := GetUserInfo(r)
		if info == nil && s.cfg.Auth.Groups.Guest {
			info = &UserInfo{Subject: "guest", Username: "guest", Groups: []string{"guest"}, Home: "/"}
			r = r.WithContext(context.WithValue(r.Context(), UserInfoKey, info))
		}
		if info == nil || !s.hasAccess(info) {
			if r.Method == http.MethodGet && wantsHTML(r) && !isBot(r) {
				http.Redirect(w, r, "/api/login?rd="+r.URL.RequestURI(), http.StatusFound)
				return
			}
			s.writeError(w, r, errors.New("unauthorized"), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		if wantsHTML(r) {
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		s.writeError(w, r, errors.New("auth not configured"), http.StatusNotImplemented)
		return
	}
	state := randomID(24)
	nonce := randomID(24)
	verifier := oauth2.GenerateVerifier()
	s.auth.PutLoginFlow(state, oidc.LoginFlow{Nonce: nonce, CodeVerifier: verifier, Created: time.Now()})

	rd := r.URL.Query().Get("rd")
	if rd != "" {
		http.SetCookie(w, &http.Cookie{Name: "godrive_rd", Value: rd, Path: "/", MaxAge: 600, HttpOnly: true})
	}
	authURL, err := s.auth.AuthorizationURL(r.Context(), state, nonce, verifier)
	if err != nil {
		s.writeError(w, r, err, http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) Callback(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.writeError(w, r, errors.New("auth not configured"), http.StatusNotImplemented)
		return
	}
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		if desc == "" {
			desc = errParam
		}
		s.writeError(w, r, fmt.Errorf("oidc: %s", desc), http.StatusBadRequest)
		return
	}
	if !oidc.AuthorizationIssValid(r.URL.Query().Get("iss"), s.cfg.Auth.Issuer, s.auth.Discovery.IssParameterSupported) {
		s.writeError(w, r, errors.New("invalid issuer"), http.StatusBadRequest)
		return
	}
	state := r.URL.Query().Get("state")
	flow, ok := s.auth.TakeLoginFlow(state)
	if !ok {
		s.writeError(w, r, errors.New("invalid state"), http.StatusBadRequest)
		return
	}

	oauth2Token, err := s.auth.Config.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(flow.CodeVerifier))
	if err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		s.writeError(w, r, errors.New("missing id_token"), http.StatusBadRequest)
		return
	}
	idToken, err := s.auth.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	if idToken.Nonce != flow.Nonce {
		s.writeError(w, r, errors.New("invalid nonce"), http.StatusBadRequest)
		return
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	// Authelia (and many IdPs) put profile/groups on UserInfo, not the ID token.
	if ui, err := s.auth.Provider.UserInfo(r.Context(), oauth2.StaticTokenSource(oauth2Token)); err == nil {
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
	home := config.ExpandHome(s.cfg.Auth.DefaultHome, username, claims.Email, idToken.Subject)
	now := time.Now().UTC()
	user, err := s.store.Q.UpsertUser(r.Context(), dbq.UpsertUserParams{
		ID:        idToken.Subject,
		Username:  username,
		Email:     claims.Email,
		Home:      home,
		Groups:    string(groupsJSON),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}

	info := s.userToInfo(user)
	if !s.hasAccess(info) {
		s.writeError(w, r, fmt.Errorf("access denied (groups=%v)", groups), http.StatusForbidden)
		return
	}
	if s.shouldProvisionHome(r.Context(), user.Home) {
		if err := s.provisionUserHome(r.Context(), user.Home, info); err != nil {
			slog.Warn("provision home failed", slog.String("path", user.Home), slog.Any("err", err))
		}
	}

	sessionID := randomID(32)
	sessionExp := now.Add(s.cfg.Auth.SessionLifespan.Duration)
	refreshExp := now.Add(s.cfg.Auth.RefreshTokenLifespan.Duration)
	_, err = s.store.Q.UpsertSession(r.Context(), dbq.UpsertSessionParams{
		ID:           sessionID,
		UserID:       user.ID,
		AccessToken:  oauth2Token.AccessToken,
		Expiry:       sessionExp,
		RefreshToken: oauth2Token.RefreshToken,
		IDToken:      rawIDToken,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}

	s.setAuthCookies(w, sessionID, sessionExp, refreshExp)

	rd := "/"
	if c, err := r.Cookie("godrive_rd"); err == nil && c.Value != "" {
		rd = c.Value
		http.SetCookie(w, &http.Cookie{Name: "godrive_rd", Value: "", Path: "/", MaxAge: -1})
	}
	http.Redirect(w, r, rd, http.StatusFound)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	var idToken string
	sessID := ""
	if c, err := r.Cookie(SessionCookieName); err == nil {
		sessID = c.Value
	}
	if sessID == "" {
		if c, err := r.Cookie(RefreshCookieName); err == nil {
			sessID = c.Value
		}
	}
	if sessID != "" {
		if sess, err := s.store.Q.GetSession(r.Context(), sessID); err == nil {
			idToken = sess.IDToken
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			s.auth.Revoke(ctx, sess.RefreshToken, "refresh_token")
			s.auth.Revoke(ctx, sess.AccessToken, "access_token")
			cancel()
			_ = s.store.Q.DeleteSession(r.Context(), sess.ID)
		} else {
			_ = s.store.Q.DeleteSession(r.Context(), sessID)
		}
	}
	s.clearAuthCookies(w)
	if r.Method == http.MethodGet {
		if loc := s.rpLogoutURL(idToken); loc != "" {
			http.Redirect(w, r, loc, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	if !s.cfg.AuthEnabled() {
		// Open mode: no OIDC — treat the operator as a local admin.
		s.writeJSON(w, map[string]any{
			"authenticated":    true,
			"auth_enabled":     false,
			"id":               info.Subject,
			"username":         info.Username,
			"home":             info.Home,
			"is_admin":         true,
			"is_access":        true,
			"is_guest":         false,
			"groups":           []string{"admin"},
			"available_groups": []string{"admin"},
		}, http.StatusOK)
		return
	}
	if info == nil {
		out := map[string]any{
			"authenticated":  false,
			"auth_enabled":   true,
			"guests_allowed": s.cfg.Auth.Groups.Guest,
		}
		if s.cfg.Auth.Groups.Guest {
			out["is_guest"] = true
			out["home"] = "/"
		}
		s.writeJSON(w, out, http.StatusOK)
		return
	}
	home := info.Home
	if s.isGuest(info) {
		home = "/"
	}
	s.writeJSON(w, map[string]any{
		"authenticated":      true,
		"auth_enabled":       true,
		"guests_allowed":     s.cfg.Auth.Groups.Guest,
		"id":                 info.Subject,
		"username":           info.Username,
		"email":              info.Email,
		"avatar":             gravatarURL(info.Email),
		"home":               home,
		"groups":             info.Groups,
		"available_groups":   s.availableGroups(),
		"is_admin":           s.isAdmin(info),
		"is_access":          s.isAccess(info),
		"is_guest":           s.isGuest(info),
		"session_expires_at": sessionExpiryJSON(GetSessionExpiry(r)),
	}, http.StatusOK)
}

func sessionExpiryJSON(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// gravatarURL matches the pre-rewrite template helper (MD5 email, retro default).
func gravatarURL(email string) string {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return ""
	}
	sum := md5.Sum([]byte(email))
	return fmt.Sprintf("https://www.gravatar.com/avatar/%x?s=80&d=retro", sum)
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func containsString(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func userIsGuest(info *UserInfo) bool {
	return info != nil && (info.Subject == "guest" || containsString(info.Groups, "guest"))
}

func (s *Server) rpLogoutURL(idToken string) string {
	if s.auth == nil || !s.cfg.AuthEnabled() {
		return ""
	}
	return s.auth.LogoutURL(idToken, s.cfg.Auth.PostLogoutRedirectURL)
}
