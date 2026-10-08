package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database/dbq"
)

func withChiStar(r *http.Request, star string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("*", star)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestPermissionsAPIAuthz(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureDir(t, "/docs", "")
	e.putACL(t, "/docs", acl.PrincipalUser, "reader", acl.PermissionRead, 0)
	e.putACL(t, "/docs", acl.PrincipalUser, "editor", acl.PermissionRead|acl.PermissionUpdatePermissions, 0)

	now := time.Now().UTC()
	for _, id := range []string{"reader", "editor", "stranger"} {
		if _, err := e.s.store.Q.UpsertUser(e.ctx, dbq.UpsertUserParams{
			ID: id, Username: id, Email: id + "@example.com", Home: "/home/" + id,
			Groups: "godrive", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}

	reader := &UserInfo{Subject: "reader", Groups: []string{"godrive"}}
	editor := &UserInfo{Subject: "editor", Groups: []string{"godrive"}}
	stranger := &UserInfo{Subject: "stranger", Groups: []string{"godrive"}}
	admin := &UserInfo{Subject: "admin", Groups: []string{"admin"}}

	t.Run("get forbidden without read", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/acl/docs", nil)
		req = withChiStar(withUser(req, stranger), "docs")
		rec := httptest.NewRecorder()
		e.s.GetPermissionsAPI(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("get ok with read", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/acl/docs", nil)
		req = withChiStar(withUser(req, reader), "docs")
		rec := httptest.NewRecorder()
		e.s.GetPermissionsAPI(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("put forbidden without update-permissions", func(t *testing.T) {
		body := `{"acl":[{"principal_type":"user","principal_id":"reader","allow":1,"deny":0}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/acl/docs", strings.NewReader(body))
		req = withChiStar(withUser(req, reader), "docs")
		rec := httptest.NewRecorder()
		e.s.PutPermissionsAPI(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("put ok with update-permissions", func(t *testing.T) {
		payload, err := json.Marshal(map[string]any{
			"acl": []map[string]any{
				{"principal_type": "user", "principal_id": "reader", "allow": int64(acl.PermissionRead), "deny": 0},
				{"principal_type": "user", "principal_id": "editor", "allow": int64(acl.PermissionRead | acl.PermissionUpdatePermissions), "deny": 0},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/api/acl/docs", bytes.NewReader(payload))
		req = withChiStar(withUser(req, editor), "docs")
		rec := httptest.NewRecorder()
		e.s.PutPermissionsAPI(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("put reserved path", func(t *testing.T) {
		body := `{"acl":[]}`
		sudoAdmin := *admin
		sudoAdmin.Sudo = true
		req := httptest.NewRequest(http.MethodPut, "/api/acl/api/me", strings.NewReader(body))
		req = withChiStar(withUser(req, &sudoAdmin), "api/me")
		rec := httptest.NewRecorder()
		e.s.PutPermissionsAPI(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("put invalid does not wipe existing", func(t *testing.T) {
		before, err := e.s.store.Q.ListACLByPath(e.ctx, "/docs")
		if err != nil || len(before) == 0 {
			t.Fatalf("setup ACL: %v %#v", err, before)
		}
		body := `{"acl":[{"principal_type":"user","principal_id":"missing-user","allow":1,"deny":0}]}`
		req := httptest.NewRequest(http.MethodPut, "/api/acl/docs", strings.NewReader(body))
		req = withChiStar(withUser(req, editor), "docs")
		rec := httptest.NewRecorder()
		e.s.PutPermissionsAPI(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		after, err := e.s.store.Q.ListACLByPath(e.ctx, "/docs")
		if err != nil || len(after) != len(before) {
			t.Fatalf("ACL wiped on failed put: before=%d after=%d err=%v", len(before), len(after), err)
		}
	})

	t.Run("patch forbidden without update-permissions", func(t *testing.T) {
		body := `{"upsert":[{"principal_type":"user","principal_id":"reader","allow":1,"deny":0}]}`
		req := httptest.NewRequest(http.MethodPatch, "/api/acl/docs", strings.NewReader(body))
		req = withChiStar(withUser(req, reader), "docs")
		rec := httptest.NewRecorder()
		e.s.PatchPermissionsAPI(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("patch upsert and remove", func(t *testing.T) {
		payload, err := json.Marshal(map[string]any{
			"upsert": []map[string]any{
				{"principal_type": "user", "principal_id": "stranger", "allow": int64(acl.PermissionRead), "deny": 0},
			},
			"remove": []map[string]any{
				{"principal_type": "user", "principal_id": "reader"},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPatch, "/api/acl/docs", bytes.NewReader(payload))
		req = withChiStar(withUser(req, editor), "docs")
		rec := httptest.NewRecorder()
		e.s.PatchPermissionsAPI(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		rows, err := e.s.store.Q.ListACLByPath(e.ctx, "/docs")
		if err != nil {
			t.Fatal(err)
		}
		var hasReader, hasStranger, hasEditor bool
		for _, row := range rows {
			switch row.PrincipalID {
			case "reader":
				hasReader = true
			case "stranger":
				hasStranger = true
			case "editor":
				hasEditor = true
			}
		}
		if hasReader || !hasStranger || !hasEditor {
			t.Fatalf("patch merge wrong: reader=%v stranger=%v editor=%v rows=%#v", hasReader, hasStranger, hasEditor, rows)
		}
	})

	t.Run("list all forbidden for non-admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings/acl", nil)
		req = withUser(req, editor)
		rec := httptest.NewRecorder()
		e.s.ListAllPermissionsAPI(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d", rec.Code)
		}
	})

	t.Run("list all ok for admin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/settings/acl", nil)
		req = withUser(req, admin)
		rec := httptest.NewRecorder()
		e.s.ListAllPermissionsAPI(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestShareAndDeleteAuthz(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureDir(t, "/docs", "")
	e.ensureFile(t, "/docs/a.txt", "", []byte("hi"))
	e.putACL(t, "/docs", acl.PrincipalUser, "reader", acl.PermissionRead, 0)
	e.putACL(t, "/docs", acl.PrincipalUser, "sharer", acl.PermissionRead|acl.PermissionShare, 0)
	e.putACL(t, "/docs/a.txt", acl.PrincipalUser, "deleter", acl.PermissionRead|acl.PermissionDelete, 0)
	e.putACL(t, "/docs/a.txt", acl.PrincipalUser, "reader", acl.PermissionRead, 0)

	reader := &UserInfo{Subject: "reader", Groups: []string{"godrive"}}
	sharer := &UserInfo{Subject: "sharer", Groups: []string{"godrive"}}
	deleter := &UserInfo{Subject: "deleter", Groups: []string{"godrive"}}

	t.Run("create share forbidden without share bit", func(t *testing.T) {
		body := `{"path":"/docs","allow":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/shares", strings.NewReader(body))
		req = withUser(req, reader)
		rec := httptest.NewRecorder()
		e.s.CreateShareAPI(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("create share ok with share bit", func(t *testing.T) {
		body := `{"path":"/docs","allow":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/shares", strings.NewReader(body))
		req = withUser(req, sharer)
		rec := httptest.NewRecorder()
		e.s.CreateShareAPI(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var out map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		id, _ := out["id"].(string)
		if id == "" {
			t.Fatal("missing share id")
		}
		// Share identity can read; wrong share cannot.
		perms, err := e.s.aclPermsFor(e.ctx, "/docs", &acl.Identity{ShareID: id})
		if err != nil || !perms.Has(acl.PermissionRead) {
			t.Fatalf("share perms %#x err=%v", perms, err)
		}
		perms, err = e.s.aclPermsFor(e.ctx, "/docs", &acl.Identity{ShareID: "nope"})
		if err != nil || perms != 0 {
			t.Fatalf("wrong share %#x err=%v", perms, err)
		}
	})

	t.Run("delete forbidden without delete bit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/docs/a.txt", bytes.NewReader([]byte("[]")))
		req = withChiStar(withUser(req, reader), "docs/a.txt")
		rec := httptest.NewRecorder()
		e.s.DeleteFilesAPI(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if _, err := e.s.store.Q.GetFile(e.ctx, "/docs/a.txt"); err != nil {
			t.Fatal("file should still exist")
		}
	})

	t.Run("delete ok with delete bit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/docs/a.txt", bytes.NewReader([]byte("[]")))
		req = withChiStar(withUser(req, deleter), "docs/a.txt")
		rec := httptest.NewRecorder()
		e.s.DeleteFilesAPI(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if _, err := e.s.store.Q.GetFile(e.ctx, "/docs/a.txt"); err == nil {
			t.Fatal("file should be gone")
		}
	})
}
