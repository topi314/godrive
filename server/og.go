package server

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"net/url"
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
		ImageURL:    "/favicon.png",
		Card:        "summary",
	}
}

// ogPublic builds Open Graph metadata for a publicly reachable path.
// publicPath is the URL path crawlers should use for media (e.g. /s/{id}/clip.jpg).
func ogPublic(filePath, description, contentType string, size int64, publicPath string) ogData {
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
	if publicPath == "" {
		publicPath = filePath
	}
	card := "summary"
	img := "/favicon.png"
	if strings.HasPrefix(contentType, "image/") {
		card = "summary_large_image"
		img = withQuery(publicPath, "preview", "1")
	}
	return ogData{
		Title:       title,
		Description: desc,
		ImageURL:    img,
		Card:        card,
	}
}

func withQuery(rawPath, key, value string) string {
	u, err := url.Parse(rawPath)
	if err != nil {
		sep := "?"
		if strings.Contains(rawPath, "?") {
			sep = "&"
		}
		return rawPath + sep + url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

func (s *Server) publicBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func htmlEscape(s string) string {
	r := strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;")
	return r.Replace(s)
}

func (s *Server) serveImagePreview(w http.ResponseWriter, r *http.Request, filePath string) {
	rc, info, err := s.storage.GetObject(r.Context(), filePath, 0, -1)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer rc.Close()
	if !strings.HasPrefix(info.ContentType, "image/") && info.ContentType != "application/octet-stream" {
		http.Redirect(w, r, "/favicon.png", http.StatusFound)
		return
	}
	src, _, err := image.Decode(rc)
	if err != nil {
		http.Redirect(w, r, "/favicon.png", http.StatusFound)
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
