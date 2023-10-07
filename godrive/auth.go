package godrive

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
)

type authKey struct{}

var UserInfoKey = authKey{}

type Auth struct {
	Verifier *oidc.IDTokenVerifier
	Config   *oauth2.Config
	Provider *oidc.Provider

	// state <-> nonce
	States   map[string]LoginState
	StatesMu sync.Mutex
}

type LoginState struct {
	Nonce       string
	RedirectURL string
}

type UserInfo struct {
	oidc.UserInfo
	Home     string   `json:"home"`
	Audience []string `json:"aud"`
	Groups   []string `json:"groups"`
	Username string   `json:"preferred_username"`
}

func (s *Server) hasFileAccess(info *UserInfo, file File) bool {
	return info.Subject == file.UserID || s.isAdmin(info)
}

func (s *Server) hasAccess(info *UserInfo) bool {
	if s.cfg.Auth == nil {
		return true
	}

	if !s.cfg.Auth.Groups.Guest && s.isGuest(info) {
		return false
	}

	return s.isAdmin(info) || s.isUser(info) || s.isViewer(info) || s.isGuest(info)
}

func (s *Server) isAdmin(info *UserInfo) bool {
	if s.cfg.Auth == nil {
		return true
	}
	return slices.Contains(info.Groups, s.cfg.Auth.Groups.Admin)
}

func (s *Server) isUser(info *UserInfo) bool {
	if s.cfg.Auth == nil {
		return false
	}
	return slices.Contains(info.Groups, s.cfg.Auth.Groups.User)
}

func (s *Server) isViewer(info *UserInfo) bool {
	if s.cfg.Auth == nil {
		return false
	}
	return slices.Contains(info.Groups, s.cfg.Auth.Groups.Viewer)
}

func (s *Server) isGuest(info *UserInfo) bool {
	return slices.Contains(info.Groups, "guest")
}

const SessionCookieName = "X-Session-ID"

func (s *Server) setSession(ctx context.Context, w http.ResponseWriter, session Session) error {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Secure:   s.cfg.Auth.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return s.db.CreateSession(ctx, session)
}

func (s *Server) removeSession(ctx context.Context, w http.ResponseWriter, sessionID string) error {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Path:     "/",
		MaxAge:   -1,
		Secure:   s.cfg.Auth.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return s.db.DeleteSession(ctx, sessionID)
}

func (s *Server) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := s.tracer.Start(r.Context(), "auth middleware")

		var sessionID string
		if cookie, err := r.Cookie(SessionCookieName); err == nil {
			sessionID = cookie.Value
			span.SetAttributes(attribute.String("sessionID", sessionID))
		}

		if sessionID == "" {
			span.End()
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		session, err := s.db.GetSession(ctx, sessionID)
		if err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to get session", slog.Any("err", err))
			_ = s.removeSession(ctx, w, sessionID)
			next.ServeHTTP(w, r)
			return
		}
		span.AddEvent("session found", trace.WithAttributes(
			attribute.String("sessionID", sessionID),
			attribute.String("accessToken", session.AccessToken),
			attribute.Stringer("expiry", session.Expiry),
			attribute.String("refreshToken", session.RefreshToken),
			attribute.String("idToken", session.IDToken),
		))

		tokenSource := s.auth.Config.TokenSource(ctx, &oauth2.Token{
			AccessToken:  session.AccessToken,
			TokenType:    "bearer",
			RefreshToken: session.RefreshToken,
			Expiry:       session.Expiry,
		})

		token, err := tokenSource.Token()
		if err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to get token", slog.Any("err", err))
			_ = s.removeSession(ctx, w, sessionID)
			span.End()
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		if token.AccessToken != session.AccessToken {
			session.AccessToken = token.AccessToken
			session.Expiry = token.Expiry
			session.RefreshToken = token.RefreshToken
			session.IDToken = token.Extra("id_token").(string)
			span.AddEvent("updating session", trace.WithAttributes(
				attribute.String("accessToken", session.AccessToken),
				attribute.Stringer("expiry", session.Expiry),
				attribute.String("refreshToken", session.RefreshToken),
				attribute.String("idToken", session.IDToken),
			))
		}

		idToken, err := s.auth.Verifier.Verify(ctx, session.IDToken)
		if err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to verify ID Token: %w", slog.Any("err", err), slog.Any("rawIDToken", session.IDToken))
			_ = s.removeSession(ctx, w, sessionID)
			span.End()
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		span.AddEvent("ID Token verified", trace.WithAttributes(attribute.String("idToken", session.IDToken)))

		var info UserInfo
		if err = idToken.Claims(&info); err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to parse claims: %w", slog.Any("err", err))
			_ = s.removeSession(ctx, w, sessionID)
			span.End()
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		span.AddEvent("claims parsed", trace.WithAttributes(
			attribute.String("subject", info.Subject),
			attribute.String("profile", info.Profile),
			attribute.String("email", info.Email),
			attribute.String("emailVerified", fmt.Sprintf("%t", info.EmailVerified)),
			attribute.String("audience", strings.Join(info.Audience, ",")),
			attribute.String("groups", strings.Join(info.Groups, ",")),
			attribute.String("username", info.Username),
		))

		user, err := s.db.GetUserByName(ctx, info.Username)
		if err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to get user by name: %w", slog.Any("err", err))
			span.End()
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		info.Home = user.Home

		span.End()
		next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, UserInfoKey, &info)))
	})
}

type AuthAction string

const (
	AuthActionDeny  AuthAction = "deny"
	AuthActionAllow AuthAction = "allow"
	AuthActionLogin AuthAction = "login"
)

func (s *Server) CheckAuth(allowedFunc func(r *http.Request, info *UserInfo) AuthAction) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch allowedFunc(r, GetUserInfo(r)) {
			case AuthActionDeny:
				s.error(w, r, errors.New("not authorized"), http.StatusForbidden)
				return

			case AuthActionLogin:
				http.Redirect(w, r, "/login", http.StatusFound)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GetUserInfo(r *http.Request) *UserInfo {
	userInfo := r.Context().Value(UserInfoKey)
	if userInfo == nil {
		return &UserInfo{
			UserInfo: oidc.UserInfo{
				Subject: "guest",
				Email:   "guest@localhost",
			},
			Audience: []string{"godrive"},
			Groups:   []string{"guest"},
			Username: "guest",
		}
	}
	return userInfo.(*UserInfo)
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	state := s.newID(16)
	nonce := s.newID(16)
	s.auth.States[state] = LoginState{
		Nonce:       nonce,
		RedirectURL: r.URL.Query().Get("rd"),
	}

	http.Redirect(w, r, s.auth.Config.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie("X-Session-ID")
	if err == nil {
		_ = s.removeSession(r.Context(), w, sessionID.Value)
	}
	logoutURL := s.cfg.Auth.LogoutURL
	if redirectURL := r.URL.Query().Get("rd"); redirectURL != "" {
		logoutURL += "?rd=" + redirectURL
	}
	http.Redirect(w, r, logoutURL, http.StatusFound)
}

func (s *Server) Callback(w http.ResponseWriter, r *http.Request) {
	ctx, span := s.tracer.Start(r.Context(), "callback")
	defer span.End()

	state := r.URL.Query().Get("state")
	loginState, ok := s.auth.States[state]
	if !ok {
		span.SetStatus(codes.Error, "invalid state")
		span.AddEvent("invalid state", trace.WithAttributes(attribute.String("state", state)))
		s.error(w, r, errors.New("invalid state"), http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	token, err := s.auth.Config.Exchange(ctx, code)
	if err != nil {
		span.SetStatus(codes.Error, "failed to exchange code")
		span.RecordError(err)
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}
	span.AddEvent("token exchanged", trace.WithAttributes(
		attribute.String("accessToken", token.AccessToken),
		attribute.Stringer("expiry", token.Expiry),
		attribute.String("refreshToken", token.RefreshToken),
	))

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		span.SetStatus(codes.Error, "no id_token in token response")
		span.AddEvent("no id_token in token response")
		s.prettyError(w, r, errors.New("no id_token in token response"), http.StatusInternalServerError)
		return
	}
	idToken, err := s.auth.Verifier.Verify(ctx, rawIDToken)
	if err != nil {
		span.SetStatus(codes.Error, "failed to verify ID Token")
		span.RecordError(err)
		s.prettyError(w, r, fmt.Errorf("failed to verify ID Token: %w", err), http.StatusInternalServerError)
		return
	}

	if idToken.Nonce != loginState.Nonce {
		span.SetStatus(codes.Error, "invalid nonce")
		span.AddEvent("invalid nonce", trace.WithAttributes(attribute.String("nonce", idToken.Nonce)))
		s.prettyError(w, r, errors.New("invalid nonce"), http.StatusBadRequest)
		return
	}

	var userInfo UserInfo
	if err = idToken.Claims(&userInfo); err != nil {
		span.SetStatus(codes.Error, "failed to parse claims")
		span.RecordError(err)
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	if !s.hasAccess(&userInfo) {
		s.prettyError(w, r, errors.New("not authorized"), http.StatusForbidden)
		return
	}

	if err = s.db.UpsertUser(ctx, idToken.Subject, userInfo.Username, userInfo.Groups, userInfo.Email, path.Join(s.cfg.Auth.DefaultHome, userInfo.Username)); err != nil {
		span.SetStatus(codes.Error, "failed to upsert user")
		span.RecordError(err)
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	sessionID := s.newID(32)
	if err = s.setSession(ctx, w, Session{
		ID:           sessionID,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
		IDToken:      rawIDToken,
	}); err != nil {
		span.SetStatus(codes.Error, "failed to set session")
	}

	http.Redirect(w, r, loginState.RedirectURL, http.StatusFound)
}
