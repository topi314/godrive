package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOgPublicImageUsesSharePreview(t *testing.T) {
	data := ogPublic("/cat.jpg", "hi", "image/jpeg", 10, "/s/abc")
	if data.ImageURL != "/s/abc?preview=1" {
		t.Fatalf("image=%q", data.ImageURL)
	}
}

func TestInjectHeadIntoSPA(t *testing.T) {
	page := []byte(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>godrive</title></head><body><div id="__nuxt"></div></body></html>`)
	out := injectHead(page, "clip.mp4", `<meta property="og:title" content="clip.mp4">`+componentEmbedScript([]byte(`{"component":{"type":17}}`)))
	if !bytes.Contains(out, []byte("<title>clip.mp4</title>")) {
		t.Fatalf("title not updated: %s", out)
	}
	if !bytes.Contains(out, []byte(`id="discord:component-embed"`)) {
		t.Fatalf("component embed missing: %s", out)
	}
	if !bytes.Contains(out, []byte(`id="__nuxt"`)) {
		t.Fatal("SPA shell missing")
	}
}

func TestBuildDiscordEmbedVideo(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/s/abc", nil)
	req.Host = "drive.example"
	raw := s.buildDiscordEmbed(req, discordEmbedInfo{
		Title:       "clip.mp4",
		Description: "concert",
		ContentType: "video/mp4",
		Size:        12 << 20,
		PageURL:     "https://drive.example/s/abc",
		MediaURL:    "https://drive.example/s/abc/clip.mp4",
	})
	if raw == nil {
		t.Fatal("expected embed")
	}
	if len(raw) > discordEmbedMax {
		t.Fatalf("embed too large: %d", len(raw))
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	comp := doc["component"].(map[string]any)
	if int(comp["type"].(float64)) != discordContainer {
		t.Fatalf("type=%v", comp["type"])
	}
	if int(comp["accent_color"].(float64)) != discordAccent {
		t.Fatalf("accent=%v", comp["accent_color"])
	}
	body := string(raw)
	if !strings.Contains(body, "clip.mp4") {
		t.Fatalf("missing title: %s", body)
	}
	if !strings.Contains(body, `"type":12`) {
		t.Fatalf("expected media gallery: %s", body)
	}
}

func TestShareMediaPath(t *testing.T) {
	if got := shareMediaPath("abc", "/kone/clip.mp4", "video/mp4"); got != "/s/abc/clip.mp4" {
		t.Fatalf("video=%q", got)
	}
	if got := shareMediaPath("abc", "/kone/cat.jpg", "image/jpeg"); got != "/s/abc/cat.jpg?preview=1" {
		t.Fatalf("image=%q", got)
	}
}

func TestWantsDiscordUnfurl(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/s/abc", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if !wantsDiscordUnfurl(req) {
		t.Fatal("expected discord unfurl")
	}
	req.Header.Set("Accept", "*/*")
	if wantsDiscordUnfurl(req) {
		t.Fatal("media fetch should stream, not unfurl")
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if wantsDiscordUnfurl(req) {
		t.Fatal("browser should not match")
	}
}

func TestIsUnsafeInlineContent(t *testing.T) {
	if !isUnsafeInlineContent("text/html; charset=utf-8", "/x.bin") {
		t.Fatal("html mime")
	}
	if !isUnsafeInlineContent("application/octet-stream", "/x.html") {
		t.Fatal("html ext")
	}
	if !isUnsafeInlineContent("image/svg+xml", "/icon") {
		t.Fatal("svg mime")
	}
	if isUnsafeInlineContent("image/png", "/cat.png") {
		t.Fatal("png should be safe inline")
	}
}

func TestWriteLinkPreviewHTML(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/s/abc", nil)
	req.Host = "drive.example"
	og, embed := s.linkPreview(req, "/clip.mp4", "hi", "video/mp4", 100, "/s/abc", "/s/abc/clip.mp4", false)
	s.writeLinkPreviewHTML(rec, req, og, embed)
	body := rec.Body.String()
	for _, want := range []string{
		`property="og:title"`,
		`id="discord:component-embed"`,
		`clip.mp4`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}
