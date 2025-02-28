package auth

import (
	"context"
	"net/http"

	"github.com/topi314/godrive/godrive/database"
	"golang.org/x/oauth2"
)

const SessionCookieName = "X-Session-ID"

type LoginState struct {
	Nonce       string
	RedirectURL string
	Verifier    string
}

func (a *Auth) NewState(redirectURL string) (string, LoginState) {
	a.statesMu.Lock()
	defer a.statesMu.Unlock()

	// ~ 90 bits to brute force
	state := a.NewID(16)
	nonce := a.NewID(16)
	verifier := oauth2.GenerateVerifier()
	l := LoginState{
		Nonce:       nonce,
		RedirectURL: redirectURL,
		Verifier:    verifier, // PKCE Verifier
	}
	a.states[state] = l

	return state, l
}

func (a *Auth) GetState(state string) (LoginState, bool) {
	a.statesMu.Lock()
	defer a.statesMu.Unlock()

	lState, ok := a.states[state]
	if ok {
		delete(a.states, state)
	}

	return lState, ok
}

func (a *Auth) NewID(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letters[a.rand.Intn(len(letters))]
	}
	return string(b)
}

func (a *Auth) NewSession(ctx context.Context, w http.ResponseWriter, session database.Session) error {
	sessionID := a.NewID(32)
	session.ID = sessionID
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		Secure:   a.cfg.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return a.db.CreateSession(ctx, session)
}

func (a *Auth) RemoveSession(ctx context.Context, w http.ResponseWriter, sessionID string) error {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Path:     "/",
		MaxAge:   -1,
		Secure:   a.cfg.Secure,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return a.db.DeleteSession(ctx, sessionID)
}
