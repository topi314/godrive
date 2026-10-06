package acl

type Permissions uint64

const (
	PermissionRead Permissions = 1 << iota
	PermissionCreate
	PermissionUpdate
	PermissionDelete
	PermissionUpdatePermissions
	PermissionShare

	PermissionsAll = PermissionRead | PermissionCreate | PermissionUpdate | PermissionDelete | PermissionUpdatePermissions | PermissionShare
)

func (p Permissions) Has(bit Permissions) bool { return p&bit == bit }
func (p Permissions) Add(bit Permissions) Permissions {
	return p | bit
}
func (p Permissions) Remove(bit Permissions) Permissions {
	return p &^ bit
}

const (
	PrincipalUser     = "user"
	PrincipalGroup    = "group"
	PrincipalEveryone = "everyone" // authenticated (non-guest) users
	PrincipalGuest    = "guest"    // unauthenticated / guest browsing
	PrincipalShare    = "share"    // capability URL /s/{id}/… only
	EveryoneID        = "*"
	GuestID           = "*"
)

type ACLRow struct {
	Path          string
	PrincipalType string
	PrincipalID   string
	Allow         Permissions
	Deny          Permissions
}

// Identity is the subject used when evaluating ACLs.
// A nil Identity is treated as an anonymous guest.
// When ShareID is set, only PrincipalShare rows for that id apply.
type Identity struct {
	Subject string
	Groups  []string
	ShareID string
}

func (id *Identity) Guest() bool {
	if id == nil {
		return true
	}
	return id.Subject == "guest" || containsString(id.Groups, "guest")
}

func CalculatePermissions(pathsRootFirst []string, rows []ACLRow, info *Identity) Permissions {
	byPath := map[string][]ACLRow{}
	for _, row := range rows {
		byPath[row.Path] = append(byPath[row.Path], row)
	}

	shareOnly := info != nil && info.ShareID != ""
	guest := info.Guest()
	var allow, deny Permissions
	for _, p := range pathsRootFirst {
		for _, row := range byPath[p] {
			switch row.PrincipalType {
			case PrincipalShare:
				if shareOnly && info.ShareID == row.PrincipalID {
					allow = allow.Add(row.Allow)
					deny = deny.Add(row.Deny)
				}
			case PrincipalEveryone:
				if !shareOnly && info != nil && !guest {
					allow = allow.Add(row.Allow)
					deny = deny.Add(row.Deny)
				}
			case PrincipalGuest:
				if !shareOnly && guest {
					allow = allow.Add(row.Allow)
					deny = deny.Add(row.Deny)
				}
			case PrincipalGroup:
				if !shareOnly && info != nil && !guest && containsString(info.Groups, row.PrincipalID) {
					allow = allow.Add(row.Allow)
					deny = deny.Add(row.Deny)
				}
			case PrincipalUser:
				if !shareOnly && info != nil && !guest && info.Subject == row.PrincipalID {
					allow = allow.Add(row.Allow)
					deny = deny.Add(row.Deny)
				}
			}
		}
	}
	return allow.Remove(deny)
}

func containsString(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
