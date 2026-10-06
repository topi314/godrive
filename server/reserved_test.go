package server

import "testing"

func TestIsReservedPath(t *testing.T) {
	reserved := []string{
		"/api", "/api/me", "/api/files/x",
		"/s", "/s/abc", "/s/abc/file.txt",
		"/_nuxt", "/_nuxt/entry.js",
		"/favicon.ico", "/favicon.png", "/favicon-light.png", "/robots.txt",
	}
	for _, p := range reserved {
		if !IsReservedPath(p) {
			t.Errorf("expected reserved: %s", p)
		}
	}
	allowed := []string{
		"/", "/home", "/version", "/ping", "/og-card.png",
		"/photos/api", "/docs/s", "/my/_nuxt", "/readme.txt",
	}
	for _, p := range allowed {
		if IsReservedPath(p) {
			t.Errorf("expected allowed: %s", p)
		}
	}
}
