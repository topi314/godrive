package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
)

func TestAdminSudoMode(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureDir(t, "/private", "alice")
	e.putACL(t, "/private", acl.PrincipalUser, "alice", acl.PermissionsAll, 0)
	// Strip inherited root read so /private stays alice-only without sudo.
	e.putACL(t, "/private", acl.PrincipalUser, "admin", 0, acl.PermissionRead)
	e.ensureDir(t, "/home/admin", "admin")
	e.putACL(t, "/home/admin", acl.PrincipalUser, "admin", acl.PermissionsAll, 0)
	e.putACL(t, "/", acl.PrincipalUser, "admin", acl.PermissionRead, 0)

	now := time.Now().UTC()
	if _, err := e.s.store.Q.UpsertUser(e.ctx, database.UpsertUserParams{
		ID: "admin", Username: "admin", Email: "admin@example.com", Home: "/home/admin",
		Groups: `["admin"]`, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("patch sudo sets cookie", func(t *testing.T) {
		admin := &UserInfo{Subject: "admin", Groups: []string{"admin"}}
		req := httptest.NewRequest(http.MethodPatch, "/api/me", strings.NewReader(`{"sudo":true}`))
		req = withUser(req, admin)
		rec := httptest.NewRecorder()
		e.s.PatchMeAPI(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var me map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
			t.Fatal(err)
		}
		if me["sudo"] != true || me["authenticated"] != true {
			t.Fatalf("patch me body=%s", rec.Body.String())
		}
		found := false
		for _, c := range rec.Result().Cookies() {
			if c.Name == SudoCookieName && c.Value == "1" {
				found = true
				if c.MaxAge != 0 {
					t.Fatalf("sudo cookie should be session-only, MaxAge=%d", c.MaxAge)
				}
			}
		}
		if !found {
			t.Fatalf("missing sudo cookie: %#v", rec.Result().Cookies())
		}
	})

	t.Run("listDir hides private without sudo", func(t *testing.T) {
		admin := &UserInfo{Subject: "admin", Groups: []string{"admin"}}
		entries, err := e.s.listDir(e.ctx, "/", admin)
		if err != nil {
			t.Fatal(err)
		}
		for _, ent := range entries {
			if ent.Path == "/private" || ent.Name == "private" {
				t.Fatalf("private visible without sudo: %#v", entries)
			}
		}
	})

	t.Run("listDir shows private with sudo", func(t *testing.T) {
		admin := &UserInfo{Subject: "admin", Groups: []string{"admin"}, Sudo: true}
		entries, err := e.s.listDir(e.ctx, "/", admin)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, ent := range entries {
			if ent.Path == "/private" || ent.Name == "private" {
				found = true
			}
		}
		if !found {
			t.Fatalf("private missing with sudo: %#v", entries)
		}
	})

	t.Run("header enables sudo in middleware", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
		req.Header.Set(SudoHeaderName, "1")
		req = withUser(req, &UserInfo{Subject: "admin", Groups: []string{"admin"}})
		// AuthMiddleware applies sudo; simulate the same gate.
		info := GetUserInfo(req)
		if e.s.isAdmin(info) && requestWantsSudo(req) {
			info.Sudo = true
		}
		if !e.s.adminSudo(info) {
			t.Fatal("expected admin sudo from header")
		}
	})
}
