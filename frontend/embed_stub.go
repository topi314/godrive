//go:build dev

package frontend

import (
	"errors"
	"io/fs"
	"net/http"
)

// Dist is unavailable in -tags dev builds (use the Nuxt dev server instead).
func Dist() (fs.FS, error) {
	return nil, errors.New("frontend not embedded in dev builds")
}

// Handler serves a short message for non-API routes when built with -tags dev.
func Handler() (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		http.Error(w, "frontend not embedded; use the Nuxt dev server (or build without -tags dev)", http.StatusNotFound)
	}), nil
}
