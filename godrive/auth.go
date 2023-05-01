package godrive

import (
	"context"
	"errors"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/exp/slices"
	"golang.org/x/oauth2"
	"net/http"
	"path"
	"time"
)

type authKey struct{}

var UserInfoKey = authKey{}

type Auth struct {
	Verifier *oidc.IDTokenVerifier
	Config   *oauth2.Config
	Provider *oidc.Provider
}

type UserInfo struct {
	oidc.UserInfo `json:"-"`
	Home          string   `json:"home"`
	Audience      []string `json:"aud"`
	Groups        []string `json:"groups"`
	Username      string   `json:"preferred_username"`
}

func (s *Server) ToTemplateUser(info *UserInfo) TemplateUser {
	return TemplateUser{
		ID:      info.Subject,
		Name:    info.Username,
		Email:   info.Email,
		Home:    info.Home,
		IsAdmin: s.isAdmin(info),
		IsUser:  s.isUser(info),
		IsGuest: s.isGuest(info),
	}
}

func (s *Server) hasFileAccess(info *UserInfo, file File) bool {
	if file.Private {
		return info.Subject == file.UserID || s.isAdmin(info)
	}
	return true
}

func (s *Server) hasAccess(info *UserInfo) bool {
	if !s.cfg.Auth.Groups.Guest && s.isGuest(info) {
		return false
	}

	return s.isAdmin(info) || s.isUser(info) || s.isViewer(info) || s.isGuest(info)
}

func (s *Server) isAdmin(info *UserInfo) bool {
	return slices.Contains(info.Groups, s.cfg.Auth.Groups.Admin)
}

func (s *Server) isUser(info *UserInfo) bool {
	return slices.Contains(info.Groups, s.cfg.Auth.Groups.User)
}

func (s *Server) isViewer(info *UserInfo) bool {
	return slices.Contains(info.Groups, s.cfg.Auth.Groups.Viewer)
}

func (s *Server) isGuest(info *UserInfo) bool {
	return slices.Contains(info.Groups, "guest")
}

func (s *Server) setCookie(w http.ResponseWriter, name string, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   s.cfg.Auth.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) removeCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		MaxAge:   -1,
		Secure:   s.cfg.Auth.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var (
			accessToken  string
			expiry       time.Time
			refreshToken string
		)
		if cookie, err := r.Cookie("access_token"); err == nil {
			accessToken = cookie.Value
			expiry = cookie.Expires
		}
		if cookie, err := r.Cookie("refresh_token"); err == nil {
			refreshToken = cookie.Value
		}

		if accessToken == "" && refreshToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		tk := s.auth.Config.TokenSource(r.Context(), &oauth2.Token{
			AccessToken:  accessToken,
			TokenType:    "bearer",
			RefreshToken: refreshToken,
			Expiry:       expiry,
		})

		userInfo, err := s.auth.Provider.UserInfo(r.Context(), tk)
		if err != nil {
			s.removeCookie(w, "access_token")
			s.removeCookie(w, "refresh_token")
			next.ServeHTTP(w, r)
			return
		}

		token, err := tk.Token()
		if err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		if token.AccessToken != accessToken || token.RefreshToken != refreshToken {
			s.setCookie(w, "access_token", token.AccessToken, token.Expiry.Sub(time.Now()))
			s.setCookie(w, "refresh_token", token.RefreshToken, 90*time.Minute)
		}

		info := new(UserInfo)
		info.UserInfo = *userInfo
		if err = userInfo.Claims(info); err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		user, err := s.db.GetUserByName(r.Context(), info.Username)
		if err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		info.Home = user.Home

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserInfoKey, info)))
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
	state := s.newID()
	nonce := s.newID()
	s.setCookie(w, "state", state, time.Minute)
	s.setCookie(w, "nonce", nonce, time.Minute)
	http.Redirect(w, r, s.auth.Config.AuthCodeURL(state, oidc.Nonce(nonce)), http.StatusFound)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	s.removeCookie(w, "access_token")
	s.removeCookie(w, "refresh_token")
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) Callback(w http.ResponseWriter, r *http.Request) {
	state, err := r.Cookie("state")
	if err != nil {
		s.prettyError(w, r, err, http.StatusBadRequest)
		return
	}
	s.removeCookie(w, "state")
	if state.Value != r.URL.Query().Get("state") {
		s.prettyError(w, r, errors.New("invalid state"), http.StatusBadRequest)
		return
	}

	token, err := s.auth.Config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		s.prettyError(w, r, errors.New("no id_token in token response"), http.StatusInternalServerError)
		return
	}
	idToken, err := s.auth.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		s.prettyError(w, r, fmt.Errorf("failed to verify ID Token: %w", err), http.StatusInternalServerError)
		return
	}

	nonce, err := r.Cookie("nonce")
	if err != nil {
		s.prettyError(w, r, err, http.StatusBadRequest)
		return
	}
	s.removeCookie(w, "nonce")
	if idToken.Nonce != nonce.Value {
		s.prettyError(w, r, errors.New("invalid nonce"), http.StatusBadRequest)
		return
	}

	var userInfo UserInfo
	if err = idToken.Claims(&userInfo); err != nil {
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	if !s.hasAccess(&userInfo) {
		s.prettyError(w, r, errors.New("not authorized"), http.StatusForbidden)
		return
	}

	s.setCookie(w, "access_token", token.AccessToken, token.Expiry.Sub(time.Now()))
	s.setCookie(w, "refresh_token", token.RefreshToken, time.Hour*24*30)

	if err = s.db.UpsertUser(r.Context(), idToken.Subject, userInfo.Username, userInfo.Email, path.Join(s.cfg.Auth.DefaultHome, userInfo.Username)); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}
