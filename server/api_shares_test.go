package server

import (
	"testing"
	"time"

	"github.com/topi314/godrive/server/database/dbq"
)

func TestParseExpiresIn(t *testing.T) {
	if got, err := parseExpiresIn("7d"); err != nil || got != 7*24*time.Hour {
		t.Fatalf("7d: %v %v", got, err)
	}
	if got, err := parseExpiresIn(""); err != nil || got != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	for _, raw := range []string{"0h", "nope", "-2d"} {
		if _, err := parseExpiresIn(raw); err == nil {
			t.Errorf("parseExpiresIn(%q) expected error", raw)
		}
	}
}

func TestShareBrowsePath(t *testing.T) {
	share := dbq.Share{ID: "xyz", Path: "/home/docs"}
	if got := shareBrowsePath(share, "/home/docs/lol2/a.pdf"); got != "/s/xyz/lol2/a.pdf" {
		t.Fatalf("got %q", got)
	}
	if got := shareBrowsePath(share, "/home/docs"); got != "/s/xyz" {
		t.Fatalf("root %q", got)
	}
}

func TestJoinShareTarget(t *testing.T) {
	root := "/home/docs"
	got, err := joinShareTarget(root, "lol2/../x")
	if err != nil || got != "/home/docs/x" {
		t.Fatalf("got %q %v", got, err)
	}
	for _, rest := range []string{"..", "../..", "foo/../../..", "..\\secret"} {
		if _, err := joinShareTarget(root, rest); err == nil {
			t.Errorf("joinShareTarget(%q) expected forbidden", rest)
		}
	}
}

func TestUnderShare(t *testing.T) {
	if underShare("/home/docs", "/home") {
		t.Fatal("parent must not be under share")
	}
	if underShare("/home/docs", "/home/docs2") {
		t.Fatal("sibling prefix must not match")
	}
}
