package server

import (
	"testing"

	"github.com/topi314/godrive/server/database/dbsqlc"
)

func TestShareBrowsePath(t *testing.T) {
	share := dbsqlc.Share{ID: "xyz", Path: "/home/docs"}
	cases := map[string]string{
		"/home/docs":            "/s/xyz",
		"/home/docs/":           "/s/xyz",
		"/home/docs/lol2":       "/s/xyz/lol2",
		"/home/docs/lol2/a.pdf": "/s/xyz/lol2/a.pdf",
	}
	for storage, want := range cases {
		if got := shareBrowsePath(share, storage); got != want {
			t.Errorf("shareBrowsePath(%q) = %q, want %q", storage, got, want)
		}
	}
}

func TestJoinShareTarget(t *testing.T) {
	root := "/home/docs"
	ok := map[string]string{
		"":           "/home/docs",
		"lol2":       "/home/docs/lol2",
		"lol2/a.pdf": "/home/docs/lol2/a.pdf",
		"lol2/../x":  "/home/docs/x",
		"./y":        "/home/docs/y",
		`lol2\x`:     "/home/docs/lol2/x", // backslash normalized to /
	}
	for rest, want := range ok {
		got, err := joinShareTarget(root, rest)
		if err != nil || got != want {
			t.Errorf("joinShareTarget(%q) = %q, %v; want %q, nil", rest, got, err, want)
		}
	}
	forbidden := []string{
		"..", "../", "../..", "../../etc",
		"..\\secret", "foo/../../..",
		"...\x00x", "%2e%2e",
	}
	for _, rest := range forbidden {
		if _, err := joinShareTarget(root, rest); err == nil {
			t.Errorf("joinShareTarget(%q) expected forbidden, got nil", rest)
		}
	}
}

func TestUnderShare(t *testing.T) {
	if !underShare("/home/docs", "/home/docs") {
		t.Fatal("root should be under itself")
	}
	if !underShare("/home/docs", "/home/docs/lol2") {
		t.Fatal("child should be under root")
	}
	if underShare("/home/docs", "/home") {
		t.Fatal("parent must not be under share")
	}
	if underShare("/home/docs", "/home/docs2") {
		t.Fatal("sibling prefix must not match")
	}
}
