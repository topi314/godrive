package server

import (
	"testing"

	"github.com/topi314/godrive/server/database"
)

func TestResolveUploadTarget(t *testing.T) {
	got, err := resolveUploadTarget("/docs", "a.pdf")
	if err != nil || got != "/docs/a.pdf" {
		t.Fatalf("rel: %q %v", got, err)
	}
	got, err = resolveUploadTarget("/docs", "inbox/a.pdf")
	if err != nil || got != "/docs/inbox/a.pdf" {
		t.Fatalf("nested: %q %v", got, err)
	}
	got, err = resolveUploadTarget("/docs", "/photos/a.jpg")
	if err != nil || got != "/photos/a.jpg" {
		t.Fatalf("abs: %q %v", got, err)
	}
	if _, err := resolveUploadTarget("/docs", "../x"); err == nil {
		t.Fatal("expected .. error")
	}
	if _, err := resolveUploadTarget("/docs", "a/"); err == nil {
		t.Fatal("expected trailing slash error")
	}
}

func TestResolveShareUploadDir(t *testing.T) {
	share := database.Share{ID: "abc", Path: "/photos"}
	if got := resolveShareUploadDir(share, ""); got != "/photos" {
		t.Fatalf("empty: %q", got)
	}
	if got := resolveShareUploadDir(share, "/s/abc"); got != "/photos" {
		t.Fatalf("root browse: %q", got)
	}
	if got := resolveShareUploadDir(share, "/s/abc/inbox"); got != "/photos/inbox" {
		t.Fatalf("nested browse: %q", got)
	}
	if got := resolveShareUploadDir(share, "/photos/inbox"); got != "/photos/inbox" {
		t.Fatalf("storage: %q", got)
	}
	if got := resolveShareUploadDir(share, "inbox"); got != "/photos/inbox" {
		t.Fatalf("relative: %q", got)
	}
}
