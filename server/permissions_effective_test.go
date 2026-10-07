package server

import (
	"testing"

	"github.com/topi314/godrive/server/acl"
)

func TestEffectivePermissions(t *testing.T) {
	e := newTestEnv(t, true)
	e.ensureDir(t, "/docs", "")
	e.ensureFile(t, "/docs/owned.txt", "alice", []byte("hi"))
	e.putACL(t, "/docs", acl.PrincipalEveryone, acl.EveryoneID, acl.PermissionRead, 0)
	e.putACL(t, "/docs", acl.PrincipalUser, "alice", acl.PermissionRead|acl.PermissionUpdate, 0)
	e.putACL(t, "/private", acl.PrincipalUser, "alice", acl.PermissionsAll, 0)
	e.ensureDir(t, "/private", "")

	alice := &UserInfo{Subject: "alice", Groups: []string{"godrive"}}
	bob := &UserInfo{Subject: "bob", Groups: []string{"godrive"}}
	admin := &UserInfo{Subject: "admin", Groups: []string{"admin"}}
	guest := &UserInfo{Subject: "guest", Groups: []string{"guest"}}

	t.Run("auth disabled grants all", func(t *testing.T) {
		s := *e.s
		s.cfg.Auth = nil
		got, err := s.EffectivePermissions(e.ctx, "/docs", bob, nil)
		if err != nil || got != acl.PermissionsAll {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})

	t.Run("admin bypass", func(t *testing.T) {
		got, err := e.s.EffectivePermissions(e.ctx, "/private", admin, nil)
		if err != nil || got != acl.PermissionsAll {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})

	t.Run("owner via ownerID bypass", func(t *testing.T) {
		owner := "bob"
		got, err := e.s.EffectivePermissions(e.ctx, "/docs", bob, &owner)
		if err != nil || got != acl.PermissionsAll {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})

	t.Run("owner via file user id", func(t *testing.T) {
		got, err := e.s.EffectivePermissions(e.ctx, "/docs/owned.txt", alice, nil)
		if err != nil || got != acl.PermissionsAll {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})

	t.Run("non-owner uses ACL", func(t *testing.T) {
		got, err := e.s.EffectivePermissions(e.ctx, "/docs", bob, nil)
		if err != nil || got != acl.PermissionRead {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})

	t.Run("acl user bits", func(t *testing.T) {
		got, err := e.s.EffectivePermissions(e.ctx, "/docs", alice, nil)
		// alice owns nothing on /docs dir; ACL grants read|update plus everyone read
		want := acl.PermissionRead | acl.PermissionUpdate
		if err != nil || got != want {
			t.Fatalf("got %#x want %#x err=%v", got, want, err)
		}
	})

	t.Run("no acl yields zero", func(t *testing.T) {
		got, err := e.s.EffectivePermissions(e.ctx, "/private", bob, nil)
		if err != nil || got != 0 {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})

	t.Run("guest uses guest ACL only", func(t *testing.T) {
		e.putACL(t, "/docs", acl.PrincipalGuest, acl.GuestID, acl.PermissionRead, 0)
		got, err := e.s.EffectivePermissions(e.ctx, "/docs", guest, nil)
		if err != nil || got != acl.PermissionRead {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})

	t.Run("anonymous denied when guest disabled", func(t *testing.T) {
		env := newTestEnv(t, false)
		env.ensureDir(t, "/docs", "")
		env.putACL(t, "/docs", acl.PrincipalGuest, acl.GuestID, acl.PermissionRead, 0)
		got, err := env.s.EffectivePermissions(env.ctx, "/docs", nil, nil)
		if err != nil || got != 0 {
			t.Fatalf("got %#x err=%v", got, err)
		}
	})
}

func TestCanAnonymousRead(t *testing.T) {
	t.Run("guest disabled", func(t *testing.T) {
		e := newTestEnv(t, false)
		e.ensureDir(t, "/docs", "")
		e.putACL(t, "/docs", acl.PrincipalGuest, acl.GuestID, acl.PermissionRead, 0)
		ok, err := e.s.CanAnonymousRead(e.ctx, "/docs")
		if err != nil || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})

	t.Run("guest enabled with read", func(t *testing.T) {
		e := newTestEnv(t, true)
		e.ensureDir(t, "/docs", "")
		e.putACL(t, "/docs", acl.PrincipalGuest, acl.GuestID, acl.PermissionRead, 0)
		ok, err := e.s.CanAnonymousRead(e.ctx, "/docs")
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})

	t.Run("guest enabled without read", func(t *testing.T) {
		e := newTestEnv(t, true)
		e.ensureDir(t, "/docs", "")
		ok, err := e.s.CanAnonymousRead(e.ctx, "/docs")
		if err != nil || ok {
			t.Fatalf("ok=%v err=%v", ok, err)
		}
	})
}
