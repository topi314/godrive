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
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/topi314/godrive/server/database/dbsqlc"
	"golang.org/x/oauth2"
)

// oidcClaims holds profile fields from an ID token and/or UserInfo response.
type oidcClaims struct {
	Email             string      `json:"email"`
	PreferredUsername string      `json:"preferred_username"`
	Groups            stringList  `json:"groups"`
}

func (c *oidcClaims) merge(o oidcClaims) {
	if c.Email == "" {
		c.Email = o.Email
	}
	if c.PreferredUsername == "" {
		c.PreferredUsername = o.PreferredUsername
	}
	if len(c.Groups) == 0 {
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

type Auth struct {
	Verifier *oidc.IDTokenVerifier
	Config   *oauth2.Config
	Provider *oidc.Provider

	States   map[string]string
	StatesMu sync.Mutex
}

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
	if s.cfg.Auth == nil {
		return true
	}
	return info != nil && containsString(info.Groups, s.cfg.Auth.Groups.Admin)
}

func (s *Server) isUser(info *UserInfo) bool {
	return info != nil && s.cfg.Auth != nil && containsString(info.Groups, s.cfg.Auth.Groups.User)
}

func (s *Server) isViewer(info *UserInfo) bool {
	return info != nil && s.cfg.Auth != nil && containsString(info.Groups, s.cfg.Auth.Groups.Viewer)
}

func (s *Server) isGuest(info *UserInfo) bool {
	return info != nil && containsString(info.Groups, "guest")
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

func (s *Server) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := s.resolveUser(r)
		if info == nil && s.cfg.Auth == nil {
			// Open mode: no OIDC — every request is a local admin.
			info = &UserInfo{Subject: "local", Username: "local", Groups: []string{"admin"}, Home: "/"}
		}
		if info != nil {
			r = r.WithContext(context.WithValue(r.Context(), UserInfoKey, info))
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) resolveUser(r *http.Request) *UserInfo {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		token := strings.TrimSpace(auth[7:])
		hash := hashToken(token)
		row, err := s.store.Q.GetAPITokenByHash(r.Context(), hash)
		if err == nil {
			user, err := s.store.Q.GetUser(r.Context(), row.UserID)
			if err == nil {
				return userToInfo(user)
			}
		}
	}

	c, err := r.Cookie(SessionCookieName)
	if err != nil || c.Value == "" {
		return nil
	}
	sess, err := s.store.Q.GetSession(r.Context(), c.Value)
	if err != nil {
		return nil
	}
	if time.Now().After(sess.Expiry) {
		_ = s.store.Q.DeleteSession(r.Context(), c.Value)
		return nil
	}
	user, err := s.store.Q.GetUser(r.Context(), sess.UserID)
	if err != nil {
		return nil
	}
	return userToInfo(user)
}

func userToInfo(u dbsqlc.User) *UserInfo {
	var groups []string
	_ = json.Unmarshal([]byte(u.Groups), &groups)
	return &UserInfo{
		Subject:  u.ID,
		Email:    u.Email,
		Home:     u.Home,
		Groups:   groups,
		Username: u.Username,
	}
}

func (s *Server) RequireAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Auth == nil {
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
	s.auth.StatesMu.Lock()
	s.auth.States[state] = nonce
	s.auth.StatesMu.Unlock()

	rd := r.URL.Query().Get("rd")
	if rd != "" {
		http.SetCookie(w, &http.Cookie{Name: "godrive_rd", Value: rd, Path: "/", MaxAge: 600, HttpOnly: true})
	}
	url := s.auth.Config.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) Callback(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil {
		s.writeError(w, r, errors.New("auth not configured"), http.StatusNotImplemented)
		return
	}
	state := r.URL.Query().Get("state")
	s.auth.StatesMu.Lock()
	nonce, ok := s.auth.States[state]
	delete(s.auth.States, state)
	s.auth.StatesMu.Unlock()
	if !ok {
		s.writeError(w, r, errors.New("invalid state"), http.StatusBadRequest)
		return
	}

	oauth2Token, err := s.auth.Config.Exchange(r.Context(), r.URL.Query().Get("code"))
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
	if idToken.Nonce != nonce {
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

	home := "/"
	if s.cfg.Auth.DefaultHome != "" {
		home = s.cfg.Auth.DefaultHome
	}
	groups := claims.Groups.Strings()
	groupsJSON, _ := json.Marshal(groups)
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Email
	}
	now := time.Now().UTC()
	user, err := s.store.Q.UpsertUser(r.Context(), dbsqlc.UpsertUserParams{
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

	info := userToInfo(user)
	if !s.hasAccess(info) {
		s.writeError(w, r, fmt.Errorf("access denied (groups=%v)", groups), http.StatusForbidden)
		return
	}

	sessionID := randomID(32)
	expiry := oauth2Token.Expiry
	if expiry.IsZero() {
		expiry = now.Add(24 * time.Hour)
	}
	_, err = s.store.Q.UpsertSession(r.Context(), dbsqlc.UpsertSessionParams{
		ID:           sessionID,
		UserID:       user.ID,
		AccessToken:  oauth2Token.AccessToken,
		Expiry:       expiry,
		RefreshToken: oauth2Token.RefreshToken,
		IDToken:      rawIDToken,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cfg.Auth.Secure,
		Expires:  expiry,
	})

	rd := "/"
	if c, err := r.Cookie("godrive_rd"); err == nil && c.Value != "" {
		rd = c.Value
		http.SetCookie(w, &http.Cookie{Name: "godrive_rd", Value: "", Path: "/", MaxAge: -1})
	}
	http.Redirect(w, r, rd, http.StatusFound)
}

func (s *Server) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(SessionCookieName); err == nil {
		_ = s.store.Q.DeleteSession(r.Context(), c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", MaxAge: -1})
	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) Me(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	if s.cfg.Auth == nil {
		// Open mode: no OIDC — treat the operator as a local admin.
		s.writeJSON(w, map[string]any{
			"authenticated": true,
			"auth_enabled":  false,
			"id":            info.Subject,
			"username":      info.Username,
			"home":          info.Home,
			"is_admin":      true,
			"is_user":       true,
			"is_viewer":     true,
			"is_guest":      false,
		}, http.StatusOK)
		return
	}
	if info == nil {
		s.writeJSON(w, map[string]any{
			"authenticated": false,
			"auth_enabled":  true,
		}, http.StatusOK)
		return
	}
	s.writeJSON(w, map[string]any{
		"authenticated": true,
		"auth_enabled":  true,
		"id":            info.Subject,
		"username":      info.Username,
		"email":         info.Email,
		"avatar":        gravatarURL(info.Email),
		"home":          info.Home,
		"groups":        info.Groups,
		"is_admin":      s.isAdmin(info),
		"is_user":       s.isUser(info),
		"is_viewer":     s.isViewer(info),
		"is_guest":      s.isGuest(info),
	}, http.StatusOK)
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

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}
