package server

import (
	"io/fs"
	"net/http"
)

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if s.public == nil {
		// -tags dev: send the browser to the Nuxt origin instead of a dead end.
		target := s.cfg.Server.FrontendURL + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}
	data, err := fs.ReadFile(s.public, "index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}
