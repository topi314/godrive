package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/storage"
)

func TestRenamePathCarriesACL(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := database.NewStore(ctx, config.DatabaseConfig{Type: config.DatabaseTypeSQLite, SQLite: config.DatabaseSQLiteConfig{Path: filepath.Join(dir, "t.db")}}, database.Migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	root := filepath.Join(dir, "files")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := storage.New(ctx, config.StorageConfig{Type: config.StorageTypeLocal, Local: config.StorageLocalConfig{Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store, storage: st}

	now := time.Now().UTC()
	if err := st.Mkdir(ctx, "/docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.UpsertFile(ctx, database.UpsertFileParams{
		Path: "/docs", Size: 0, ContentType: storage.ContentTypeDirectory, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.UpsertFile(ctx, database.UpsertFileParams{
		Path: "/docs/a.txt", Size: 2, ContentType: "text/plain", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.UpsertACL(ctx, database.UpsertACLParams{
		Path: "/docs", PrincipalType: acl.PrincipalEveryone, PrincipalID: acl.EveryoneID, Allow: int64(acl.PermissionsAll),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.UpsertACL(ctx, database.UpsertACLParams{
		Path: "/docs/a.txt", PrincipalType: acl.PrincipalUser, PrincipalID: "u1", Allow: int64(acl.PermissionRead),
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.renamePath(ctx, "/docs", "/notes", nil); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Q.GetFile(ctx, "/docs"); err == nil {
		t.Fatal("old folder still in db")
	}
	if _, err := store.Q.GetFile(ctx, "/notes"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.GetFile(ctx, "/notes/a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "notes", "a.txt")); err != nil {
		t.Fatal(err)
	}

	folderACL, err := store.Q.ListACLByPath(ctx, "/notes")
	if err != nil || len(folderACL) != 1 {
		t.Fatalf("folder ACL: %v %#v", err, folderACL)
	}
	fileACL, err := store.Q.ListACLByPath(ctx, "/notes/a.txt")
	if err != nil || len(fileACL) != 1 || fileACL[0].PrincipalID != "u1" {
		t.Fatalf("file ACL: %v %#v", err, fileACL)
	}
	if old, _ := store.Q.ListACLByPath(ctx, "/docs"); len(old) != 0 {
		t.Fatalf("old folder ACL remains: %#v", old)
	}
}

func TestRenamePathMovesDeeperAndUp(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := database.NewStore(ctx, config.DatabaseConfig{Type: config.DatabaseTypeSQLite, SQLite: config.DatabaseSQLiteConfig{Path: filepath.Join(dir, "t.db")}}, database.Migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := filepath.Join(dir, "files")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := storage.New(ctx, config.StorageConfig{Type: config.StorageTypeLocal, Local: config.StorageLocalConfig{Path: root}})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store, storage: st}
	now := time.Now().UTC()
	if err := st.Mkdir(ctx, "/docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.UpsertFile(ctx, database.UpsertFileParams{
		Path: "/docs", Size: 0, ContentType: storage.ContentTypeDirectory, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.UpsertFile(ctx, database.UpsertFileParams{
		Path: "/docs/a.txt", Size: 2, ContentType: "text/plain", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	to, err := resolveRenameTarget("/docs/a.txt", "archive/nested/a.txt")
	if err != nil || to != "/docs/archive/nested/a.txt" {
		t.Fatalf("deeper target: %q %v", to, err)
	}
	if err := s.renamePath(ctx, "/docs/a.txt", to, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.GetFile(ctx, "/docs/archive"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.GetFile(ctx, "/docs/archive/nested"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "archive", "nested", "a.txt")); err != nil {
		t.Fatal(err)
	}

	up, err := resolveRenameTarget("/docs/archive/nested/a.txt", "../../../other/a.txt")
	if err != nil || up != "/other/a.txt" {
		t.Fatalf("up target: %q %v", up, err)
	}
	if err := s.renamePath(ctx, "/docs/archive/nested/a.txt", up, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.GetFile(ctx, "/other"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "other", "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Q.GetFile(ctx, "/other/a.txt"); err != nil {
		t.Fatal(err)
	}
}
