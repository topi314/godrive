package godrive

import (
	"context"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"net/http"
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
	Audience      []string `json:"aud"`
	Groups        []string `json:"groups"`
	Username      string   `json:"preferred_username"`
}

func (s *Server) setCookie(w http.ResponseWriter, name string, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		Secure:   s.cfg.Auth.Secure,
		HttpOnly: true,
	})
}

func (s *Server) removeCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Path:     "/",
		MaxAge:   -1,
		Secure:   s.cfg.Auth.Secure,
		HttpOnly: true,
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
		} else {
			next.ServeHTTP(w, r)
			return
		}

		if cookie, err := r.Cookie("refresh_token"); err == nil {
			refreshToken = cookie.Value
		}

		userInfo, err := s.auth.Provider.UserInfo(r.Context(), oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken:  accessToken,
			TokenType:    "bearer",
			RefreshToken: refreshToken,
			Expiry:       expiry,
		}))

		info := new(UserInfo)
		info.UserInfo = *userInfo
		if err = userInfo.Claims(info); err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserInfoKey, info)))
	})
}

func (s *Server) CheckAuth(allowedFunc func(r *http.Request, info *UserInfo) bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !allowedFunc(r, GetUserInfo(r)) {
				s.error(w, r, errors.New("not authorized"), http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func GetUserInfo(r *http.Request) *UserInfo {
	userInfo := r.Context().Value(UserInfoKey)
	if userInfo == nil {
		return nil
	}
	return userInfo.(*UserInfo)
}

func (s *Server) Login(w http.ResponseWriter, r *http.Request) {
	state := s.newID()
	nonce := s.newID()
	s.setCookie(w, "state", state, time.Hour)
	s.setCookie(w, "nonce", nonce, time.Hour)
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
		s.error(w, r, err, http.StatusBadRequest)
		return
	}
	s.removeCookie(w, "state")
	if state.Value != r.URL.Query().Get("state") {
		s.error(w, r, errors.New("invalid state"), http.StatusBadRequest)
		return
	}

	token, err := s.auth.Config.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "No id_token field in oauth2 token.", http.StatusInternalServerError)
		return
	}
	idToken, err := s.auth.Verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "Failed to verify ID Token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	nonce, err := r.Cookie("nonce")
	if err != nil {
		http.Error(w, "nonce not found", http.StatusBadRequest)
		return
	}
	s.removeCookie(w, "nonce")
	if idToken.Nonce != nonce.Value {
		http.Error(w, "nonce did not match", http.StatusBadRequest)
		return
	}

	s.setCookie(w, "access_token", token.AccessToken, token.Expiry.Sub(time.Now()))
	s.setCookie(w, "refresh_token", token.RefreshToken, time.Hour*24*30)

	var userInfo UserInfo
	if err = idToken.Claims(&userInfo); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	if err = s.db.UpsertUser(r.Context(), idToken.Subject, userInfo.Username); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}
