package acl

import "testing"

func TestLikeContains(t *testing.T) {
	cases := []struct{ in, want string }{
		{"report", "%report%"},
		{"a%b_c", "%abc%"},
		{"*.mp3", "%.mp3%"},
		{"report*", "%report%"},
		{"test(*).mp3", "%test(%).mp3%"},
		{"?oot", "%_oot%"},
		{"*foo?bar*", "%foo_bar%"},
		{"***", "%%%"},
	}
	for _, tc := range cases {
		if got := LikeContains(tc.in); got != tc.want {
			t.Errorf("LikeContains(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

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
