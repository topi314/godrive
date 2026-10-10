package acl

import "testing"

func TestIsReservedPath(t *testing.T) {
	// Prefix collisions: names that contain reserved segments must still be allowed.
	reserved := []string{"/api/me", "/s/abc/file.txt", "/_nuxt/entry.js", "/favicon.ico", "/.uploads", "/.uploads/u_abc"}
	for _, p := range reserved {
		if !IsReservedPath(p) {
			t.Errorf("expected reserved: %s", p)
		}
	}
	allowed := []string{"/", "/home", "/photos/api", "/docs/s", "/my/_nuxt", "/readme.txt", "/my/.uploads"}
	for _, p := range allowed {
		if IsReservedPath(p) {
			t.Errorf("expected allowed: %s", p)
		}
	}
}
