package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/topi314/godrive/server/acl"
)

// Private and missing paths must be indistinguishable (no existence leak).
func TestPathEnumerationHidden(t *testing.T) {
	e := newTestEnv(t, false)
	e.ensureDir(t, "/public", "")
	e.ensureDir(t, "/secret", "alice")
	e.ensureFile(t, "/secret/hidden.txt", "alice", []byte("x"))
	e.putACL(t, "/public", acl.PrincipalEveryone, acl.EveryoneID, acl.PermissionRead, 0)
	e.putACL(t, "/secret", acl.PrincipalUser, "alice", acl.PermissionsAll, 0)

	bob := &UserInfo{Subject: "bob", Groups: []string{"godrive"}}

	t.Run("get json identical", func(t *testing.T) {
		get := func(path string) (int, map[string]any) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Accept", "application/json")
			req = withUser(req, bob)
			rec := httptest.NewRecorder()
			e.s.GetPath(rec, req)
			var body map[string]any
			_ = json.Unmarshal(rec.Body.Bytes(), &body)
			return rec.Code, body
		}
		hCode, hBody := get("/secret/hidden.txt")
		mCode, mBody := get("/secret/nope.txt")
		if hCode != http.StatusNotFound || mCode != http.StatusNotFound {
			t.Fatalf("codes hidden=%d missing=%d", hCode, mCode)
		}
		if hBody["message"] != "not found" || mBody["message"] != "not found" {
			t.Fatalf("messages %#v %#v", hBody["message"], mBody["message"])
		}
		if hBody["status"] != mBody["status"] {
			t.Fatalf("status field %#v %#v", hBody["status"], mBody["status"])
		}
	})

	t.Run("get html identical og", func(t *testing.T) {
		get := func(path string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Accept", "text/html")
			req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)")
			req = withUser(req, bob)
			rec := httptest.NewRecorder()
			e.s.GetPath(rec, req)
			return rec
		}
		hidden := get("/secret/hidden.txt")
		missing := get("/secret/nope.txt")
		if hidden.Code != http.StatusOK || missing.Code != http.StatusOK {
			t.Fatalf("codes hidden=%d missing=%d", hidden.Code, missing.Code)
		}
		h := hidden.Body.String()
		m := missing.Body.String()
		if !strings.Contains(h, "This link is unavailable.") || !strings.Contains(m, "This link is unavailable.") {
			t.Fatalf("expected unavailable OG\nhidden=%s\nmissing=%s", h, m)
		}
		// Strip path-specific absolute URLs; remaining markup must match.
		norm := func(s, path string) string {
			s = strings.ReplaceAll(s, path, "/__path__")
			return s
		}
		if norm(h, "/secret/hidden.txt") != norm(m, "/secret/nope.txt") {
			t.Fatalf("html differs\nhidden=%s\nmissing=%s", h, m)
		}
	})

	t.Run("missing under readable parent still 404", func(t *testing.T) {
		// Inherited read must not make missing names look different from private ones.
		req := httptest.NewRequest(http.MethodGet, "/public/guess.txt", nil)
		req.Header.Set("Accept", "application/json")
		req = withUser(req, bob)
		rec := httptest.NewRecorder()
		e.s.GetPath(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		req = httptest.NewRequest(http.MethodGet, "/api/acl/public/guess.txt", nil)
		req = withChiStar(withUser(req, bob), "public/guess.txt")
		rec = httptest.NewRecorder()
		e.s.GetPermissionsAPI(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("acl status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("visible but undeletable is 403", func(t *testing.T) {
		e.putACL(t, "/secret/hidden.txt", acl.PrincipalUser, "bob", acl.PermissionRead, 0)
		req := httptest.NewRequest(http.MethodDelete, "/secret/hidden.txt", nil)
		req = withChiStar(withUser(req, bob), "secret/hidden.txt")
		rec := httptest.NewRecorder()
		e.s.DeleteFilesAPI(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("delete without bit status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("acl probe private folder is 404", func(t *testing.T) {
		carol := &UserInfo{Subject: "carol", Groups: []string{"godrive"}}
		req := httptest.NewRequest(http.MethodGet, "/api/acl/secret", nil)
		req = withChiStar(withUser(req, carol), "secret")
		rec := httptest.NewRecorder()
		e.s.GetPermissionsAPI(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("acl probe status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}
