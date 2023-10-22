package auth

import (
	"context"
	"net/http"

	"github.com/topi314/godrive/godrive/database"
)

const SessionCookieName = "X-Session-ID"

type loginState struct {
	Nonce       string
	RedirectURL string
}

func (a *Auth) NewState(redirectURL string) (string, string) {
	a.statesMu.Lock()
	defer a.statesMu.Unlock()

	state := a.NewID(16)
	nonce := a.NewID(16)
	a.states[state] = loginState{
		Nonce:       nonce,
		RedirectURL: redirectURL,
	}

	return state, nonce
}

func (a *Auth) GetState(state string) (string, string, bool) {
	a.statesMu.Lock()
	defer a.statesMu.Unlock()

	lState, ok := a.states[state]
	if ok {
		delete(a.states, state)
	}

	return lState.Nonce, lState.RedirectURL, ok
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
