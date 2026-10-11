package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/topi314/godrive/server/acl"
)

func TestGetPathJSONVariesAccept(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureDir(t, "/home/alice", "alice")
	e.putACL(t, "/home/alice", acl.PrincipalUser, "alice", acl.PermissionsAll, 0)

	alice := &UserInfo{Subject: "alice", Groups: []string{"godrive"}}
	req := httptest.NewRequest(http.MethodGet, "/home/alice", nil)
	req.Header.Set("Accept", "application/json")
	req = withUser(req, alice)
	rec := httptest.NewRecorder()
	e.s.GetPath(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("Content-Type=%q", ct)
	}
	if !headerHas(rec.Header(), "Vary", "Accept") {
		t.Fatalf("Vary=%q, want Accept", rec.Header().Values("Vary"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("Cache-Control=%q, want no-store", cc)
	}
}

func headerHas(h http.Header, key, token string) bool {
	for _, v := range h.Values(key) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}
