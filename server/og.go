package server

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"path"
	"strings"

	"golang.org/x/image/draw"
)

type ogData struct {
	Title       string
	Description string
	ImageURL    string
	Card        string
	URL         string
}

func ogPrivate() ogData {
	return ogData{
		Title:       "godrive",
		Description: "Private file on godrive",
		ImageURL:    "/api/og-card.png",
		Card:        "summary",
	}
}

func ogPublic(filePath, description, contentType string, size int64) ogData {
	title := path.Base(filePath)
	if title == "/" || title == "." || title == "" {
		title = "godrive"
	}
	desc := description
	if desc == "" {
		if contentType != "" {
			desc = contentType
		} else {
			desc = "Shared on godrive"
		}
		if size > 0 {
			desc = fmt.Sprintf("%s · %s", desc, humanBytes(size))
		}
	}
	card := "summary"
	img := "/api/og-card.png"
	if strings.HasPrefix(contentType, "image/") {
		card = "summary_large_image"
		img = filePath + "?preview=1"
	}
	return ogData{
		Title:       title,
		Description: desc,
		ImageURL:    img,
		Card:        card,
	}
}

func (s *Server) writeOG(w http.ResponseWriter, r *http.Request, data ogData) {
	if data.URL == "" {
		data.URL = r.URL.RequestURI()
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	base := scheme + "://" + r.Host
	img := data.ImageURL
	if strings.HasPrefix(img, "/") {
		img = base + img
	}
	canon := base + data.URL

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>%s</title>
<meta property="og:title" content="%s">
<meta property="og:description" content="%s">
<meta property="og:url" content="%s">
<meta property="og:type" content="website">
<meta property="og:image" content="%s">
<meta name="twitter:card" content="%s">
<meta name="twitter:title" content="%s">
<meta name="twitter:description" content="%s">
<meta name="twitter:image" content="%s">
</head><body><p>%s</p></body></html>`,
		htmlEscape(data.Title),
		htmlEscape(data.Title), htmlEscape(data.Description), htmlEscape(canon), htmlEscape(img),
		htmlEscape(data.Card),
		htmlEscape(data.Title), htmlEscape(data.Description), htmlEscape(img),
		htmlEscape(data.Description),
	)
}

func htmlEscape(s string) string {
	r := strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;")
	return r.Replace(s)
}

func isBot(r *http.Request) bool {
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	bots := []string{
		"bot", "slack", "twitter", "facebook", "discord", "linkedin", "telegram",
		"whatsapp", "preview", "crawler", "spider", "embedly", "quora", "pinterest",
		"reddit", "vkshare", "w3c_validator", "applebot", "bingpreview",
	}
	for _, b := range bots {
		if strings.Contains(ua, b) {
			return true
		}
	}
	return false
}

func (s *Server) serveImagePreview(w http.ResponseWriter, r *http.Request, filePath string) {
	rc, info, err := s.storage.GetObject(r.Context(), filePath, 0, -1)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	if !strings.HasPrefix(info.ContentType, "image/") && info.ContentType != "application/octet-stream" {
		http.Redirect(w, r, "/api/og-card.png", http.StatusFound)
		return
	}
	src, _, err := image.Decode(rc)
	if err != nil {
		http.Redirect(w, r, "/api/og-card.png", http.StatusFound)
		return
	}
	const max = 1200
	b := src.Bounds()
	w0, h0 := b.Dx(), b.Dy()
	if w0 > max || h0 > max {
		scale := float64(max) / float64(w0)
		if h0 > w0 {
			scale = float64(max) / float64(h0)
		}
		nw := int(float64(w0) * scale)
		nh := int(float64(h0) * scale)
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
		src = dst
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(buf.Bytes())
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
