package server

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request, og *ogData, embed []byte) {
	if s.public == nil {
		// -tags dev: send the browser to the Nuxt origin instead of a dead end.
		target := s.cfg.Server.FrontendURL + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}
	data, err := fs.ReadFile(s.public, "index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusServiceUnavailable)
		return
	}
	if og != nil || len(embed) > 0 {
		title, meta := "", ""
		if og != nil {
			title, meta = s.ogHead(r, *og)
		}
		if len(embed) > 0 {
			meta += componentEmbedScript(embed)
		}
		data = injectHead(data, title, meta)
	}
	varyAccept(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

// ogHead returns <title> + Open Graph / Twitter meta tags for injection into index.html.
func (s *Server) ogHead(r *http.Request, data ogData) (title string, meta string) {
	if data.URL == "" {
		data.URL = r.URL.RequestURI()
		if u, err := url.Parse(data.URL); err == nil {
			q := u.Query()
			q.Del("preview")
			u.RawQuery = q.Encode()
			data.URL = u.String()
		}
	}
	base := s.publicBase(r)
	img := absPublicURL(base, data.ImageURL)
	canon := absPublicURL(base, data.URL)
	title = htmlEscape(data.Title)
	var b strings.Builder
	fmt.Fprintf(&b, `<meta property="og:title" content="%s">`, title)
	fmt.Fprintf(&b, `<meta property="og:description" content="%s">`, htmlEscape(data.Description))
	fmt.Fprintf(&b, `<meta property="og:url" content="%s">`, htmlEscape(canon))
	b.WriteString(`<meta property="og:type" content="website">`)
	fmt.Fprintf(&b, `<meta property="og:image" content="%s">`, htmlEscape(img))
	fmt.Fprintf(&b, `<meta name="twitter:card" content="%s">`, htmlEscape(data.Card))
	fmt.Fprintf(&b, `<meta name="twitter:title" content="%s">`, title)
	fmt.Fprintf(&b, `<meta name="twitter:description" content="%s">`, htmlEscape(data.Description))
	fmt.Fprintf(&b, `<meta name="twitter:image" content="%s">`, htmlEscape(img))
	return title, b.String()
}

func injectHead(page []byte, title, meta string) []byte {
	if title != "" {
		if bytes.Contains(page, []byte("<title>godrive</title>")) {
			page = bytes.Replace(page, []byte("<title>godrive</title>"), []byte("<title>"+title+"</title>"), 1)
		} else if i := bytes.Index(page, []byte("<title>")); i >= 0 {
			if j := bytes.Index(page[i:], []byte("</title>")); j >= 0 {
				page = append(page[:i], append([]byte("<title>"+title+"</title>"), page[i+j+8:]...)...)
			}
		}
	}
	if meta == "" {
		return page
	}
	head := []byte("<head>")
	if i := bytes.Index(page, head); i >= 0 {
		i += len(head)
		out := make([]byte, 0, len(page)+len(meta))
		out = append(out, page[:i]...)
		out = append(out, meta...)
		out = append(out, page[i:]...)
		return out
	}
	return page
}
