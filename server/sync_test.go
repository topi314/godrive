package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/database/dbq"
	"github.com/topi314/godrive/server/storage"
)

func TestSyncPrefixPreservesUpdatedAt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := database.NewStore(ctx, config.DatabaseConfig{
		Type: config.DatabaseTypeSQLite, SQLite: config.DatabaseSQLiteConfig{Path: filepath.Join(dir, "t.db")},
	}, database.Migrations)
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

	path := filepath.Join(root, "a.txt")
	if err := os.WriteFile(path, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	// Deliberately different MIME than list/stat will report — refresh must not rewrite.
	if _, err := store.Q.UpsertFile(ctx, dbq.UpsertFileParams{
		Path: "/a.txt", Size: 2, ContentType: "application/x-custom", CreatedAt: stamp, UpdatedAt: stamp,
	}); err != nil {
		t.Fatal(err)
	}

	s.syncPrefix(ctx, "/")
	s.syncPrefix(ctx, "/")

	got, err := store.Q.GetFile(ctx, "/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !got.UpdatedAt.Equal(stamp) {
		t.Fatalf("UpdatedAt changed on reconcile: got %v want %v", got.UpdatedAt, stamp)
	}
	if got.ContentType != "application/x-custom" {
		t.Fatalf("ContentType changed on reconcile: got %q", got.ContentType)
	}
}
