package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
)

func (s *Server) writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error, status int) {
	slog.ErrorContext(r.Context(), "request error", slog.String("path", r.URL.Path), slog.Any("err", err))
	s.writeJSON(w, map[string]any{
		"message": err.Error(),
		"status":  status,
		"path":    r.URL.Path,
	}, status)
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}
