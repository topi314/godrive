package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/storage"
)

func TestHasAccessRoles(t *testing.T) {
	s := &Server{cfg: config.Config{Auth: &config.AuthConfig{Groups: config.AuthGroups{
		Admin: "admin", Access: "godrive", Guest: true,
	}}}}
	admin := &UserInfo{Subject: "a", Groups: []string{"admin"}}
	user := &UserInfo{Subject: "u", Groups: []string{"godrive"}}
	guest := &UserInfo{Subject: "guest", Groups: []string{"guest"}}
	nobody := &UserInfo{Subject: "x", Groups: []string{"vpn"}}
	if !s.isAdmin(admin) || s.isAccess(admin) {
		t.Fatal("admin role")
	}
	if !s.hasAccess(admin) || !s.hasAccess(user) || !s.hasAccess(guest) {
		t.Fatal("expected access")
	}
	if s.hasAccess(nobody) {
		t.Fatal("unmapped groups should not grant access")
	}
}

func TestResolveRenameTarget(t *testing.T) {
	got, err := resolveRenameTarget("/docs/a.txt", "../b.txt")
	if err != nil || got != "/b.txt" {
		t.Fatalf("got %q %v", got, err)
	}
	for _, spec := range []string{"..", "/", ""} {
		if _, err := resolveRenameTarget("/docs/a.txt", spec); err == nil {
			t.Errorf("resolve(%q) expected error", spec)
		}
	}
}

func TestProvisionUserHome(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := database.NewStore(ctx, config.DatabaseConfig{Type: config.DatabaseTypeSQLite, Path: filepath.Join(dir, "t.db")}, database.Migrations)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	root := filepath.Join(dir, "files")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := storage.New(ctx, config.StorageConfig{Type: config.StorageTypeLocal, Path: root})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{store: store, storage: st, cfg: config.Config{Auth: &config.AuthConfig{}}}
	info := &UserInfo{Subject: "u1", Username: "alice"}

	if err := s.provisionUserHome(ctx, "/home/alice", info); err != nil {
		t.Fatal(err)
	}

	file, err := store.Q.GetFile(ctx, "/home/alice")
	if err != nil || !file.UserID.Valid || file.UserID.String != "u1" {
		t.Fatalf("home owner: %#v %v", file, err)
	}
	if _, err := os.Stat(filepath.Join(root, "home", "alice")); err != nil {
		t.Fatal(err)
	}
	rules, err := store.Q.ListACLByPath(ctx, "/home/alice")
	if err != nil || len(rules) != 1 || rules[0].PrincipalType != acl.PrincipalUser || rules[0].PrincipalID != "u1" || rules[0].Allow != int64(acl.PermissionsAll) {
		t.Fatalf("home ACL: %#v %v", rules, err)
	}

	perms, err := s.EffectivePermissions(ctx, "/home/alice", info, nil)
	if err != nil || perms != acl.PermissionsAll {
		t.Fatalf("owner perms: %v %v", perms, err)
	}

	if err := s.provisionUserHome(ctx, "/home/alice", info); err != nil {
		t.Fatal(err)
	}
	rules, _ = store.Q.ListACLByPath(ctx, "/home/alice")
	if len(rules) != 1 {
		t.Fatalf("ACL should stay seeded once: %#v", rules)
	}

	other := &UserInfo{Subject: "u2", Username: "bob"}
	if err := s.provisionUserHome(ctx, "/home/alice", other); err != nil {
		t.Fatal(err)
	}
	file, _ = store.Q.GetFile(ctx, "/home/alice")
	if file.UserID.String != "u1" {
		t.Fatalf("must not steal home: %#v", file)
	}
}
