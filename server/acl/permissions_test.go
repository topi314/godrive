package acl

import "testing"

func TestCalculatePermissions(t *testing.T) {
	alice := &Identity{Subject: "alice", Groups: []string{"editors", "godrive"}}
	bob := &Identity{Subject: "bob", Groups: []string{"godrive"}}
	guest := &Identity{Subject: "guest", Groups: []string{"guest"}}
	shareABC := &Identity{ShareID: "abc"}

	tests := []struct {
		name  string
		paths []string
		rows  []ACLRow
		id    *Identity
		want  Permissions
	}{
		{
			name:  "nil identity uses guest rules",
			paths: []string{"/"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
				{Path: "/", PrincipalType: PrincipalGuest, PrincipalID: GuestID, Allow: PermissionRead},
			},
			id:   nil,
			want: PermissionRead,
		},
		{
			name:  "logged-in gets everyone not guest",
			paths: []string{"/"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
				{Path: "/", PrincipalType: PrincipalGuest, PrincipalID: GuestID, Allow: PermissionRead},
			},
			id:   alice,
			want: PermissionsAll,
		},
		{
			name:  "guest does not inherit everyone",
			paths: []string{"/"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
				{Path: "/", PrincipalType: PrincipalGuest, PrincipalID: GuestID, Allow: PermissionRead},
			},
			id:   guest,
			want: PermissionRead,
		},
		{
			name:  "user principal match",
			paths: []string{"/", "/home/alice"},
			rows: []ACLRow{
				{Path: "/home/alice", PrincipalType: PrincipalUser, PrincipalID: "alice", Allow: PermissionRead | PermissionUpdate},
			},
			id:   alice,
			want: PermissionRead | PermissionUpdate,
		},
		{
			name:  "user principal mismatch",
			paths: []string{"/", "/home/alice"},
			rows: []ACLRow{
				{Path: "/home/alice", PrincipalType: PrincipalUser, PrincipalID: "alice", Allow: PermissionsAll},
			},
			id:   bob,
			want: 0,
		},
		{
			name:  "group principal match",
			paths: []string{"/", "/docs"},
			rows: []ACLRow{
				{Path: "/docs", PrincipalType: PrincipalGroup, PrincipalID: "editors", Allow: PermissionRead | PermissionCreate},
			},
			id:   alice,
			want: PermissionRead | PermissionCreate,
		},
		{
			name:  "group principal mismatch",
			paths: []string{"/", "/docs"},
			rows: []ACLRow{
				{Path: "/docs", PrincipalType: PrincipalGroup, PrincipalID: "editors", Allow: PermissionsAll},
			},
			id:   bob,
			want: 0,
		},
		{
			name:  "deny removes overlapping allow",
			paths: []string{"/"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll, Deny: PermissionDelete | PermissionShare},
			},
			id:   alice,
			want: PermissionRead | PermissionCreate | PermissionUpdate | PermissionUpdatePermissions,
		},
		{
			name:  "child deny overrides parent allow",
			paths: []string{"/", "/secret"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
				{Path: "/secret", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Deny: PermissionRead},
			},
			id:   alice,
			want: PermissionsAll.Remove(PermissionRead),
		},
		{
			name:  "inheritance accumulates across ancestors",
			paths: []string{"/", "/a", "/a/b"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionRead},
				{Path: "/a", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionCreate},
				{Path: "/a/b", PrincipalType: PrincipalUser, PrincipalID: "alice", Allow: PermissionUpdate},
			},
			id:   alice,
			want: PermissionRead | PermissionCreate | PermissionUpdate,
		},
		{
			name:  "share identity ignores everyone and user rows",
			paths: []string{"/", "/photos"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
				{Path: "/photos", PrincipalType: PrincipalUser, PrincipalID: "alice", Allow: PermissionsAll},
				{Path: "/photos", PrincipalType: PrincipalShare, PrincipalID: "abc", Allow: PermissionRead | PermissionCreate},
			},
			id:   shareABC,
			want: PermissionRead | PermissionCreate,
		},
		{
			name:  "wrong share id gets nothing",
			paths: []string{"/", "/photos"},
			rows: []ACLRow{
				{Path: "/", PrincipalType: PrincipalEveryone, PrincipalID: EveryoneID, Allow: PermissionsAll},
				{Path: "/photos", PrincipalType: PrincipalShare, PrincipalID: "abc", Allow: PermissionRead},
			},
			id:   &Identity{ShareID: "other"},
			want: 0,
		},
		{
			name:  "normal user ignores share rows",
			paths: []string{"/", "/photos"},
			rows: []ACLRow{
				{Path: "/photos", PrincipalType: PrincipalShare, PrincipalID: "abc", Allow: PermissionsAll},
			},
			id:   alice,
			want: 0,
		},
		{
			name:  "guest identity ignores user and group rows",
			paths: []string{"/", "/docs"},
			rows: []ACLRow{
				{Path: "/docs", PrincipalType: PrincipalUser, PrincipalID: "alice", Allow: PermissionsAll},
				{Path: "/docs", PrincipalType: PrincipalGroup, PrincipalID: "editors", Allow: PermissionsAll},
				{Path: "/docs", PrincipalType: PrincipalGuest, PrincipalID: GuestID, Allow: PermissionRead},
			},
			id:   guest,
			want: PermissionRead,
		},
		{
			name:  "no matching rows yields zero",
			paths: []string{"/", "/empty"},
			rows:  nil,
			id:    alice,
			want:  0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalculatePermissions(tc.paths, tc.rows, tc.id)
			if got != tc.want {
				t.Fatalf("got %#x (%v) want %#x (%v)", got, got, tc.want, tc.want)
			}
		})
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
		{"/x", "/", "/root", "/root/x"},
	}
	for _, tc := range tests {
		if got := RemapPrefix(tc.p, tc.from, tc.to); got != tc.want {
			t.Fatalf("remap(%q, %q, %q) = %q want %q", tc.p, tc.from, tc.to, got, tc.want)
		}
	}
}

func TestPermissionsHasAddRemove(t *testing.T) {
	p := PermissionRead.Add(PermissionCreate)
	if !p.Has(PermissionRead) || !p.Has(PermissionCreate) || p.Has(PermissionDelete) {
		t.Fatalf("unexpected bits %#x", p)
	}
	p = p.Remove(PermissionRead)
	if p.Has(PermissionRead) || !p.Has(PermissionCreate) {
		t.Fatalf("after remove %#x", p)
	}
}
