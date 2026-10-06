package acl

import "testing"

func TestCalculatePermissionsEveryoneVsGuest(t *testing.T) {
	paths := []string{"/"}
	rows := []ACLRow{
		{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
		{Path: "/", PrincipalType: PrincipalGuest, PrincipalID: GuestID, Allow: PermissionRead},
	}

	loggedIn := &Identity{Subject: "u1", Groups: []string{"godrive"}}
	if got := CalculatePermissions(paths, rows, loggedIn); got != PermissionsAll {
		t.Fatalf("logged-in: got %#x want %#x", got, PermissionsAll)
	}

	guest := &Identity{Subject: "guest", Groups: []string{"guest"}}
	if got := CalculatePermissions(paths, rows, guest); got != PermissionRead {
		t.Fatalf("guest: got %#x want %#x", got, PermissionRead)
	}

	if got := CalculatePermissions(paths, rows, nil); got != PermissionRead {
		t.Fatalf("anonymous: got %#x want %#x", got, PermissionRead)
	}
}

func TestCalculatePermissionsShareOnly(t *testing.T) {
	paths := []string{"/", "/photos"}
	rows := []ACLRow{
		{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
		{Path: "/photos", PrincipalType: PrincipalShare, PrincipalID: "abc", Allow: PermissionCreate},
	}
	// Normal user ignores share rows.
	user := &Identity{Subject: "u1", Groups: []string{"godrive"}}
	if got := CalculatePermissions(paths, rows, user); got != PermissionsAll {
		t.Fatalf("user: got %#x", got)
	}
	// Share identity only gets share bits.
	share := &Identity{ShareID: "abc"}
	if got := CalculatePermissions(paths, rows, share); got != PermissionCreate {
		t.Fatalf("share: got %#x want create", got)
	}
	other := &Identity{ShareID: "other"}
	if got := CalculatePermissions(paths, rows, other); got != 0 {
		t.Fatalf("other share: got %#x", got)
	}
}

func TestRemapPrefix(t *testing.T) {
	tests := []struct {
		p, from, to, want string
	}{
		{"/docs", "/docs", "/notes", "/notes"},
		{"/docs/a.txt", "/docs", "/notes", "/notes/a.txt"},
		{"/docs-old", "/docs", "/notes", "/docs-old"},
		{"/a.txt", "/a.txt", "/b.txt", "/b.txt"},
	}
	for _, tc := range tests {
		if got := RemapPrefix(tc.p, tc.from, tc.to); got != tc.want {
			t.Fatalf("remap(%q, %q, %q) = %q want %q", tc.p, tc.from, tc.to, got, tc.want)
		}
	}
}
