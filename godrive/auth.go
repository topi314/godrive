package godrive

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/topi314/godrive/godrive/auth"
	"github.com/topi314/godrive/godrive/database"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
)

func (s *Server) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := s.tracer.Start(r.Context(), "auth middleware")

		var sessionID string
		if cookie, err := r.Cookie(auth.SessionCookieName); err == nil {
			sessionID = cookie.Value
			span.SetAttributes(attribute.String("session_id", sessionID))
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
			_ = s.auth.RemoveSession(ctx, w, sessionID)
			next.ServeHTTP(w, r)
			return
		}
		span.AddEvent("session found", trace.WithAttributes(
			attribute.String("session_id", sessionID),
			attribute.String("access_token", session.AccessToken),
			attribute.Stringer("expiry", session.Expiry),
			attribute.String("refresh_token", session.RefreshToken),
			attribute.String("id_token", session.IDToken),
		))

		tokenSource := s.auth.Config().TokenSource(ctx, &oauth2.Token{
			AccessToken:  session.AccessToken,
			TokenType:    "bearer",
			RefreshToken: session.RefreshToken,
			Expiry:       session.Expiry,
		})

		token, err := tokenSource.Token()
		if err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to get token", slog.Any("err", err))
			_ = s.auth.RemoveSession(ctx, w, sessionID)
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
				attribute.String("access_token", session.AccessToken),
				attribute.Stringer("expiry", session.Expiry),
				attribute.String("refresh_token", session.RefreshToken),
				attribute.String("id_token", session.IDToken),
			))
		}

		idToken, err := s.auth.Verifier().Verify(ctx, session.IDToken)
		if err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to verify ID Token: %w", slog.Any("err", err), slog.Any("rawIDToken", session.IDToken))
			_ = s.auth.RemoveSession(ctx, w, sessionID)
			span.End()
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		span.AddEvent("ID Token verified", trace.WithAttributes(attribute.String("id_token", session.IDToken)))

		var info auth.UserInfo
		if err = idToken.Claims(&info); err != nil {
			span.RecordError(err)
			slog.ErrorContext(ctx, "failed to parse claims: %w", slog.Any("err", err))
			_ = s.auth.RemoveSession(ctx, w, sessionID)
			span.End()
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		span.AddEvent("claims parsed", trace.WithAttributes(
			attribute.String("subject", info.Subject),
			attribute.String("profile", info.Profile),
			attribute.String("email", info.Email),
			attribute.String("email_verified", fmt.Sprintf("%t", info.EmailVerified)),
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
		next.ServeHTTP(w, auth.SetUserInfo(r, &info))
	})
}

func (s *Server) CheckAuth(allowedFunc func(r *http.Request, info *auth.UserInfo) auth.Action) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch allowedFunc(r, auth.GetUserInfo(r)) {
			case auth.ActionDeny:
				s.error(w, r, errors.New("not authorized"), http.StatusForbidden)
				return

			case auth.ActionLogin:
				http.Redirect(w, r, "/api/login", http.StatusFound)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	state, nonce := s.auth.NewState(r.URL.Query().Get("rd"))
	http.Redirect(w, r, s.auth.Config().AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	sessionID, err := r.Cookie("X-Session-ID")
	if err == nil {
		_ = s.auth.RemoveSession(r.Context(), w, sessionID.Value)
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
	nonce, redirectURL, ok := s.auth.GetState(state)
	if !ok {
		span.SetStatus(codes.Error, "invalid state")
		span.AddEvent("invalid state", trace.WithAttributes(attribute.String("state", state)))
		s.error(w, r, errors.New("invalid state"), http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	token, err := s.auth.Config().Exchange(ctx, code)
	if err != nil {
		span.SetStatus(codes.Error, "failed to exchange code")
		span.RecordError(err)
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}
	span.AddEvent("token exchanged", trace.WithAttributes(
		attribute.String("access_token", token.AccessToken),
		attribute.Stringer("expiry", token.Expiry),
		attribute.String("refresh_token", token.RefreshToken),
	))

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		span.SetStatus(codes.Error, "no id_token in token response")
		span.AddEvent("no id_token in token response")
		s.prettyError(w, r, errors.New("no id_token in token response"), http.StatusInternalServerError)
		return
	}
	idToken, err := s.auth.Verifier().Verify(ctx, rawIDToken)
	if err != nil {
		span.SetStatus(codes.Error, "failed to verify ID Token")
		span.RecordError(err)
		s.prettyError(w, r, fmt.Errorf("failed to verify ID Token: %w", err), http.StatusInternalServerError)
		return
	}

	if idToken.Nonce != nonce {
		span.SetStatus(codes.Error, "invalid nonce")
		span.AddEvent("invalid nonce", trace.WithAttributes(attribute.String("nonce", idToken.Nonce)))
		s.prettyError(w, r, errors.New("invalid nonce"), http.StatusBadRequest)
		return
	}

	var userInfo auth.UserInfo
	if err = idToken.Claims(&userInfo); err != nil {
		span.SetStatus(codes.Error, "failed to parse claims")
		span.RecordError(err)
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	if !s.auth.HasAccess(&userInfo) {
		s.prettyError(w, r, errors.New("not authorized"), http.StatusForbidden)
		return
	}

	if err = s.db.UpsertUser(ctx, idToken.Subject, userInfo.Username, userInfo.Groups, userInfo.Email, path.Join(s.cfg.Auth.DefaultHome, userInfo.Username)); err != nil {
		span.SetStatus(codes.Error, "failed to upsert user")
		span.RecordError(err)
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	if err = s.auth.NewSession(ctx, w, database.Session{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		Expiry:       token.Expiry,
		IDToken:      rawIDToken,
	}); err != nil {
		span.SetStatus(codes.Error, "failed to set session")
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}
