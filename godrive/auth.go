package godrive

import (
	"context"
	"errors"
	"fmt"
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
	Claims map[string]any
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
		cookie, err := r.Cookie("access_token")
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		_, err = s.auth.Verifier.Verify(r.Context(), cookie.Value)
		if err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		claims := make(map[string]any)
		//if err = token.Claims(&claims); err != nil {
		//	s.error(w, r, err, http.StatusInternalServerError)
		//	return
		//}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserInfoKey, &UserInfo{
			Claims: claims,
		})))
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

	fmt.Println("token", token.AccessToken)
	fmt.Println("rawIDToken", rawIDToken)
	claims := make(map[string]any)
	idToken.Claims(&claims)
	fmt.Printf("claims: %+v\n", claims)

	s.setCookie(w, "access_token", rawIDToken, token.Expiry.Sub(time.Now()))
	s.setCookie(w, "refresh_token", token.RefreshToken, time.Hour*24*30)

	http.Redirect(w, r, "/", http.StatusFound)
}
