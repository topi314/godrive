package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/topi314/godrive/server/storage"
)

// Discord component types for link-preview embeds.
// https://docs.discord.com/developers/link-previews/component-embeds
const (
	discordButton       = 2
	discordSection      = 9
	discordTextDisplay  = 10
	discordMediaGallery = 12
	discordSeparator    = 14
	discordContainer    = 17

	discordButtonLink = 5
	discordAccent     = 0xe100ff // matches --accent in the SPA
	discordEmbedMax   = 3000
)

type discordEmbedInfo struct {
	Title       string
	Description string
	ContentType string
	Size        int64
	PageURL     string
	MediaURL    string
	IsDir       bool
	Private     bool
}

func wantsDiscordUnfurl(r *http.Request) bool {
	return isDiscordbot(r) && wantsHTML(r) && !wantsDownload(r)
}

func (s *Server) buildDiscordEmbed(r *http.Request, info discordEmbedInfo) []byte {
	page := info.PageURL
	if page == "" {
		page = s.publicBase(r) + r.URL.Path
	}
	title := strings.TrimSpace(info.Title)
	if title == "" {
		title = "godrive"
	}

	var body strings.Builder
	if info.Private {
		body.WriteString("# Unavailable\nThis link is not publicly readable.")
	} else {
		fmt.Fprintf(&body, "# **[%s](%s)**\n", escapeDiscordMDLink(title), page)
		switch {
		case info.IsDir:
			body.WriteString("Folder")
		case info.ContentType != "":
			body.WriteString(info.ContentType)
		default:
			body.WriteString("File")
		}
		if info.Size > 0 && !info.IsDir {
			fmt.Fprintf(&body, " · %s", humanBytes(info.Size))
		}
		if d := strings.TrimSpace(info.Description); d != "" {
			body.WriteString("\n")
			body.WriteString(trimEmbedText(d, 200))
		}
	}

	children := []map[string]any{
		{
			"type": discordSection,
			"components": []map[string]any{
				{"type": discordTextDisplay, "content": trimEmbedText(body.String(), 500)},
			},
			"accessory": map[string]any{
				"type": discordButton, "style": discordButtonLink, "url": page, "label": "Open",
			},
		},
	}
	if !info.Private && info.MediaURL != "" && discordMediaOK(info.ContentType, info.MediaURL) {
		children = append(children,
			map[string]any{"type": discordSeparator, "spacing": 1, "divider": true},
			map[string]any{
				"type": discordMediaGallery,
				"items": []map[string]any{
					{"media": map[string]any{"url": info.MediaURL}},
				},
			},
		)
	}

	raw, err := json.Marshal(map[string]any{
		"component": map[string]any{
			"type": discordContainer, "accent_color": discordAccent, "components": children,
		},
	})
	if err != nil {
		return nil
	}
	if len(raw) <= discordEmbedMax {
		return raw
	}
	// Shrink to a title + Open button if we blew the 3KB limit.
	raw, err = json.Marshal(map[string]any{
		"component": map[string]any{
			"type": discordContainer, "accent_color": discordAccent,
			"components": []map[string]any{{
				"type": discordSection,
				"components": []map[string]any{
					{"type": discordTextDisplay, "content": trimEmbedText("# **" + escapeDiscordMDLink(title) + "**\nShared on godrive", 200)},
				},
				"accessory": map[string]any{
					"type": discordButton, "style": discordButtonLink, "url": page, "label": "Open",
				},
			}},
		},
	})
	if err != nil || len(raw) > discordEmbedMax {
		return nil
	}
	return raw
}

func discordMediaOK(contentType, mediaURL string) bool {
	ct := strings.ToLower(contentType)
	u := strings.ToLower(mediaURL)
	switch {
	case strings.HasPrefix(ct, "image/"):
		return hasAnySuffix(u, ".png", ".gif", ".jpg", ".jpeg", ".webp", ".avif") ||
			strings.Contains(u, "preview=1")
	case strings.HasPrefix(ct, "video/"):
		return hasAnySuffix(u, ".mp4", ".webm", ".mov")
	default:
		return false
	}
}

func hasAnySuffix(s string, suffixes ...string) bool {
	if i := strings.IndexByte(s, '?'); i >= 0 {
		s = s[:i]
	}
	for _, suf := range suffixes {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

func escapeDiscordMDLink(s string) string {
	return strings.NewReplacer("[", "\\[", "]", "\\]").Replace(s)
}

func trimEmbedText(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max-1]) + "…"
}

// shareMediaPath is a public URL Discord can fetch as media (basename keeps a file extension).
func shareMediaPath(shareID, storagePath, contentType string) string {
	base := "/s/" + shareID
	if name := path.Base(storagePath); name != "/" && name != "." && name != "" {
		base += "/" + name
	}
	return mediaPublicPath(base, contentType)
}

func mediaPublicPath(publicPath, contentType string) string {
	if strings.HasPrefix(contentType, "image/") {
		return withQuery(publicPath, "preview", "1")
	}
	return publicPath
}

func absPublicURL(base, p string) string {
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
		return p
	}
	if strings.HasPrefix(p, "/") {
		return base + p
	}
	return base + "/" + p
}

func componentEmbedScript(raw []byte) string {
	safe := strings.ReplaceAll(string(raw), "<", `\u003c`)
	return `<script id="discord:component-embed" type="application/json">` + safe + `</script>`
}

// linkPreview bundles OG + Discord component-embed data for a path.
func (s *Server) linkPreview(r *http.Request, filePath, description, contentType string, size int64, pagePath, mediaPath string, private bool) (ogData, []byte) {
	base := s.publicBase(r)
	pageURL := absPublicURL(base, pagePath)
	if private {
		return ogPrivate(), s.buildDiscordEmbed(r, discordEmbedInfo{
			Title: "godrive", Private: true, PageURL: pageURL,
		})
	}
	og := ogPublic(filePath, description, contentType, size, pagePath)
	if mediaPath != "" && strings.HasPrefix(contentType, "image/") {
		og.ImageURL = mediaPath
	}
	embed := s.buildDiscordEmbed(r, discordEmbedInfo{
		Title:       path.Base(filePath),
		Description: description,
		ContentType: contentType,
		Size:        size,
		PageURL:     pageURL,
		MediaURL:    absPublicURL(base, mediaPath),
		IsDir:       storage.IsDirectory(contentType),
	})
	return og, embed
}

// serveLinkPreview sends OG + component-embed HTML.
// Discordbot and -tags destin (no embedded SPA) get a minimal document — destin must
// not 307 to FrontendURL for the same path, or Vite's file proxy loops forever.
// Production browsers get the embedded SPA with OG injected.
func (s *Server) serveLinkPreview(w http.ResponseWriter, r *http.Request, og ogData, embed []byte) {
	if isDiscordbot(r) || s.public == nil {
		s.writeLinkPreviewHTML(w, r, og, embed)
		return
	}
	s.serveSPA(w, r, &og, embed)
}

func (s *Server) writeLinkPreviewHTML(w http.ResponseWriter, r *http.Request, og ogData, embed []byte) {
	title, meta := s.ogHead(r, og)
	if len(embed) > 0 {
		meta += componentEmbedScript(embed)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html><head>
<meta charset="utf-8">
<title>%s</title>
%s
</head><body><p>%s</p></body></html>`,
		title, meta, htmlEscape(og.Description),
	)
}
