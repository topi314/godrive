package config

import "testing"

func TestExpandHome(t *testing.T) {
	if got := ExpandHome("/home/{user}", "alice", "", ""); got != "/home/alice" {
		t.Fatalf("got %q", got)
	}
	if got := ExpandHome("/home/{user}", "../etc", "ok@x", "id"); got != "/home/ok@x" {
		t.Fatalf("unsafe user fell back to email: %q", got)
	}
}
