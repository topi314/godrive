package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
)

func TestPatchFileOwner(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureFile(t, "/docs/a.txt", "alice", []byte("hi"))

	now := time.Now().UTC()
	for _, id := range []string{"alice", "bob"} {
		if _, err := e.s.store.Q.UpsertUser(e.ctx, database.UpsertUserParams{
			ID: id, Username: id, Email: id + "@example.com", Home: "/home/" + id,
			Groups: "godrive", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.s.store.Q.UpsertACL(e.ctx, database.UpsertACLParams{
		Path: "/", PrincipalType: acl.PrincipalEveryone, PrincipalID: acl.EveryoneID,
		Allow: int64(acl.PermissionRead | acl.PermissionUpdate),
	}); err != nil {
		t.Fatal(err)
	}

	patch := func(user *UserInfo, ownerID string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]any{"owner_id": ownerID})
		req := httptest.NewRequest(http.MethodPatch, "/docs/a.txt", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req = withUser(req, user)
		rr := httptest.NewRecorder()
		e.s.PatchFileAPI(rr, req)
		return rr
	}

	rr := patch(&UserInfo{Subject: "bob", Groups: []string{"godrive"}}, "bob")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("bob without A: got %d", rr.Code)
	}

	rr = patch(&UserInfo{Subject: "alice", Groups: []string{"godrive"}}, "bob")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("alice transfer: got %d %s", rr.Code, rr.Body.String())
	}
	file, err := e.s.store.Q.GetFile(e.ctx, "/docs/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !file.UserID.Valid || file.UserID.String != "bob" {
		t.Fatalf("owner=%v", file.UserID)
	}

	rr = patch(&UserInfo{Subject: "bob", Groups: []string{"godrive"}}, "missing")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("unknown owner: got %d", rr.Code)
	}
}

func TestPatchFileOwnerRecursive(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureDir(t, "/docs", "alice")
	e.ensureFile(t, "/docs/a.txt", "alice", []byte("a"))
	e.ensureFile(t, "/docs/sub/b.txt", "alice", []byte("b"))
	e.ensureFile(t, "/other/c.txt", "alice", []byte("c"))

	now := time.Now().UTC()
	for _, id := range []string{"alice", "bob"} {
		if _, err := e.s.store.Q.UpsertUser(e.ctx, database.UpsertUserParams{
			ID: id, Username: id, Email: id + "@example.com", Home: "/home/" + id,
			Groups: "godrive", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.s.store.Q.UpsertACL(e.ctx, database.UpsertACLParams{
		Path: "/", PrincipalType: acl.PrincipalEveryone, PrincipalID: acl.EveryoneID,
		Allow: int64(acl.PermissionRead | acl.PermissionUpdatePermissions),
	}); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"owner_id": "bob", "owner_recursive": true})
	req := httptest.NewRequest(http.MethodPatch, "/docs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, &UserInfo{Subject: "alice", Groups: []string{"godrive"}})
	rr := httptest.NewRecorder()
	e.s.PatchFileAPI(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("recursive transfer: got %d %s", rr.Code, rr.Body.String())
	}

	for _, p := range []string{"/docs", "/docs/a.txt", "/docs/sub/b.txt"} {
		file, err := e.s.store.Q.GetFile(e.ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		if !file.UserID.Valid || file.UserID.String != "bob" {
			t.Fatalf("%s owner=%v want bob", p, file.UserID)
		}
	}
	other, err := e.s.store.Q.GetFile(e.ctx, "/other/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !other.UserID.Valid || other.UserID.String != "alice" {
		t.Fatalf("sibling owner changed: %v", other.UserID)
	}
}
