package godrive

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/godrive/db"
)

func (s *Server) ListTokensAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	tokens, err := s.store.Q.ListAPITokensByUser(r.Context(), info.Subject)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, len(tokens))
	for i, t := range tokens {
		out[i] = map[string]any{
			"token_prefix": t.TokenPrefix,
			"token_hash":   t.TokenHash,
			"description":  t.Description,
			"created_at":   t.CreatedAt,
		}
	}
	s.writeJSON(w, out, http.StatusOK)
}

func (s *Server) CreateTokenAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	var body struct {
		Description string `json:"description"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := hashToken(token)
	prefix := token
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	now := time.Now().UTC()
	row, err := s.store.Q.CreateAPIToken(r.Context(), db.CreateAPITokenParams{
		TokenHash: hash, TokenPrefix: prefix, UserID: info.Subject,
		Description: body.Description, CreatedAt: now,
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{
		"token":        token,
		"token_prefix": row.TokenPrefix,
		"token_hash":   row.TokenHash,
		"description":  row.Description,
		"created_at":   row.CreatedAt,
	}, http.StatusCreated)
}

func (s *Server) DeleteTokenAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	hash := chi.URLParam(r, "hash")
	if err := s.store.Q.DeleteAPIToken(r.Context(), db.DeleteAPITokenParams{
		TokenHash: hash, UserID: info.Subject,
	}); err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
