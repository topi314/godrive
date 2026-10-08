package server

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/database/dbq"
	"github.com/topi314/godrive/server/storage"
)

type testEnv struct {
	s   *Server
	ctx context.Context
}

func newTestEnv(t *testing.T, guest bool) *testEnv {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	store, err := database.NewStore(ctx, config.DatabaseConfig{
		Type:   config.DatabaseTypeSQLite,
		SQLite: config.DatabaseSQLiteConfig{Path: filepath.Join(dir, "t.db")},
	}, database.Migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	root := filepath.Join(dir, "files")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := storage.New(ctx, config.StorageConfig{
		Type:  config.StorageTypeLocal,
		Local: config.StorageLocalConfig{Path: root},
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &Server{
		store:   store,
		storage: st,
		cfg: config.Config{
			Auth: &config.AuthConfig{
				Enabled: true,
				Groups: config.AuthGroups{
					Admin:  "admin",
					Access: "godrive",
					Guest:  guest,
					Map:    map[string]string{"editors": "editors"},
				},
			},
			Upload: config.UploadConfig{
				MaxSize:   config.ByteSize{Bytes: 50 << 30},
				ChunkSize: config.ByteSize{Bytes: 16 << 20},
			},
		},
	}
	return &testEnv{s: s, ctx: ctx}
}

func (e *testEnv) ensureDir(t *testing.T, p, owner string) {
	t.Helper()
	p = acl.NormalizePath(p)
	now := time.Now().UTC()
	for _, anc := range acl.AncestorPathsRootFirst(p) {
		if anc == "/" {
			continue
		}
		if err := e.s.storage.Mkdir(e.ctx, anc); err != nil {
			t.Fatal(err)
		}
		if _, err := e.s.store.Q.GetFile(e.ctx, anc); err == nil {
			continue
		}
		ownerID := ""
		if anc == p {
			ownerID = owner
		}
		if _, err := e.s.store.Q.UpsertFile(e.ctx, dirFileParams(anc, now, ownerID)); err != nil {
			t.Fatal(err)
		}
	}
}

func (e *testEnv) ensureFile(t *testing.T, p, owner string, body []byte) {
	t.Helper()
	p = acl.NormalizePath(p)
	e.ensureDir(t, path.Dir(p), "")
	now := time.Now().UTC()
	if err := e.s.storage.PutObject(e.ctx, p, int64(len(body)), bytes.NewReader(body), "text/plain"); err != nil {
		t.Fatal(err)
	}
	params := dbq.UpsertFileParams{
		Path: p, Size: int64(len(body)), ContentType: "text/plain",
		CreatedAt: now, UpdatedAt: now,
	}
	if owner != "" {
		params.UserID = database.NullString(&owner)
	}
	if _, err := e.s.store.Q.UpsertFile(e.ctx, params); err != nil {
		t.Fatal(err)
	}
}

func (e *testEnv) putACL(t *testing.T, p, principalType, principalID string, allow, deny acl.Permissions) {
	t.Helper()
	if _, err := e.s.store.Q.UpsertACL(e.ctx, dbq.UpsertACLParams{
		Path: p, PrincipalType: principalType, PrincipalID: principalID,
		Allow: int64(allow), Deny: int64(deny),
	}); err != nil {
		t.Fatal(err)
	}
}

func withUser(r *http.Request, info *UserInfo) *http.Request {
	if info == nil {
		return r
	}
	return r.WithContext(context.WithValue(r.Context(), UserInfoKey, info))
}
