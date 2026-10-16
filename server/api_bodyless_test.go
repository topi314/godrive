package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/topi314/godrive/server/acl"
)

func TestBodylessFileOps(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureDir(t, "/home/alice", "alice")
	e.ensureFile(t, "/home/alice/note.txt", "alice", []byte("hi"))
	e.putACL(t, "/home/alice", acl.PrincipalUser, "alice", acl.PermissionsAll, 0)

	alice := &UserInfo{Subject: "alice", Groups: []string{"godrive"}}

	t.Run("mkdir path no body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/home/alice/docs", nil)
		req = withChiStar(withUser(req, alice), "home/alice/docs")
		rec := httptest.NewRecorder()
		e.s.UploadFileAPI(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if _, err := e.s.store.Q.GetFile(e.ctx, "/home/alice/docs"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("put raw file body", func(t *testing.T) {
		body := []byte("hello raw upload")
		req := httptest.NewRequest(http.MethodPost, "/home/alice/docs/hello.txt", bytes.NewReader(body))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("Content-Length", strconv.Itoa(len(body)))
		req.ContentLength = int64(len(body))
		req = withChiStar(withUser(req, alice), "home/alice/docs/hello.txt")
		rec := httptest.NewRecorder()
		e.s.UploadFileAPI(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		file, err := e.s.store.Q.GetFile(e.ctx, "/home/alice/docs/hello.txt")
		if err != nil || file.Size != int64(len(body)) {
			t.Fatalf("file %#v err=%v", file, err)
		}
	})

	t.Run("delete path no body", func(t *testing.T) {
		e.ensureFile(t, "/home/alice/trash.txt", "alice", []byte("x"))
		req := httptest.NewRequest(http.MethodDelete, "/home/alice/trash.txt", nil)
		req = withChiStar(withUser(req, alice), "home/alice/trash.txt")
		rec := httptest.NewRecorder()
		e.s.DeleteFilesAPI(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if _, err := e.s.store.Q.GetFile(e.ctx, "/home/alice/trash.txt"); err == nil {
			t.Fatal("expected deleted")
		}
	})

	t.Run("delete batch children", func(t *testing.T) {
		e.ensureFile(t, "/home/alice/a.txt", "alice", []byte("a"))
		e.ensureFile(t, "/home/alice/b.txt", "alice", []byte("b"))
		e.ensureDir(t, "/home/alice/old", "alice")
		req := httptest.NewRequest(http.MethodDelete, "/home/alice", bytes.NewReader([]byte(`["a.txt","b.txt","old"]`)))
		req.Header.Set("Content-Type", "application/json")
		req = withChiStar(withUser(req, alice), "home/alice")
		rec := httptest.NewRecorder()
		e.s.DeleteFilesAPI(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		for _, p := range []string{"/home/alice/a.txt", "/home/alice/b.txt", "/home/alice/old"} {
			if _, err := e.s.store.Q.GetFile(e.ctx, p); err == nil {
				t.Fatalf("expected deleted %s", p)
			}
		}
		if _, err := e.s.store.Q.GetFile(e.ctx, "/home/alice"); err != nil {
			t.Fatal("parent should remain")
		}
	})

	t.Run("move path no body", func(t *testing.T) {
		e.ensureDir(t, "/home/alice/archive", "alice")
		e.ensureFile(t, "/home/alice/move-me.txt", "alice", []byte("m"))
		req := httptest.NewRequest(http.MethodPut, "/home/alice/move-me.txt", nil)
		req.Header.Set("Destination", "/home/alice/archive")
		req = withChiStar(withUser(req, alice), "home/alice/move-me.txt")
		rec := httptest.NewRecorder()
		e.s.MoveFilesAPI(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if _, err := e.s.store.Q.GetFile(e.ctx, "/home/alice/archive/move-me.txt"); err != nil {
			t.Fatal(err)
		}
	})
}
