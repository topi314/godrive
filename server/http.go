package server

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

func (s *Server) writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// varyAccept marks responses that differ by Accept (HTML SPA vs JSON listing).
// Without this, browsers/CDNs may reuse a cached JSON list as the document after
// session restore (same URL, navigation Accept: text/html).
func varyAccept(w http.ResponseWriter) {
	for _, v := range w.Header().Values("Vary") {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "Accept") {
				return
			}
		}
	}
	w.Header().Add("Vary", "Accept")
}

// writeNegotiatedJSON writes JSON for a content-negotiated path URL and prevents
// caches from storing it as the sole representation of that URL.
func (s *Server) writeNegotiatedJSON(w http.ResponseWriter, v any, status int) {
	varyAccept(w)
	w.Header().Set("Cache-Control", "private, no-store")
	s.writeJSON(w, v, status)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error, status int) {
	slog.ErrorContext(r.Context(), "request error", slog.String("path", r.URL.Path), slog.Any("err", err))
	w.Header().Set("Cache-Control", "private, no-store")
	s.writeJSON(w, map[string]any{
		"message": err.Error(),
		"status":  status,
		"path":    r.URL.Path,
	}, status)
}

func wantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// decodeJSONOptional decodes JSON from the request body. An empty or missing
// body is success and leaves dst unchanged (zero value / prior contents).
func decodeJSONOptional(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}
