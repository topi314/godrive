package godrive

import (
	"io/fs"
	"net/http"
)

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if s.public == nil {
		http.Error(w, "frontend not built", http.StatusServiceUnavailable)
		return
	}
	publicFS := s.public
	if sub, err := fs.Sub(s.public, "public"); err == nil {
		publicFS = sub
	}
	data, err := fs.ReadFile(publicFS, "index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}
