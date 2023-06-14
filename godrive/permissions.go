package godrive

import (
	"encoding/json"
	"net/http"
	"strings"

	"golang.org/x/exp/slices"
)

type ObjectType int

func (o ObjectType) MarshalJSON() ([]byte, error) {
	return json.Marshal(AllObjectTypes[o])
}

func (o *ObjectType) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	for ot, name := range AllObjectTypes {
		if str == name {
			*o = ot
			return nil
		}
	}
	return nil
}

func (o ObjectType) String() string {
	return AllObjectTypes[o]
}

var AllObjectTypes = map[ObjectType]string{
	ObjectTypeUser:  "user",
	ObjectTypeGroup: "group",
}

const (
	ObjectTypeUnknown ObjectType = iota
	ObjectTypeUser
	ObjectTypeGroup
)

var AllPermissions = map[Permissions]string{
	PermissionRead:   "read",
	PermissionWrite:  "write",
	PermissionCreate: "create",
	PermissionDelete: "delete",
	PermissionShare:  "share",
}

type Permissions int

const (
	PermissionRead Permissions = 1 << iota
	PermissionWrite
	PermissionCreate
	PermissionDelete
	PermissionShare
	PermissionsNone = 0
	PermissionsAll  = PermissionRead | PermissionWrite | PermissionCreate | PermissionDelete | PermissionShare
)

func (p Permissions) MarshalJSON() ([]byte, error) {
	var perms []string
	for perm, name := range AllPermissions {
		if p.Has(perm) {
			perms = append(perms, name)
		}
	}
	return json.Marshal(perms)
}

func (p *Permissions) UnmarshalJSON(data []byte) error {
	var perms []string
	if err := json.Unmarshal(data, &perms); err != nil {
		return err
	}
	for _, perm := range perms {
		for ap, name := range AllPermissions {
			if perm == name {
				*p = p.Add(ap)
			}
		}
	}
	return nil
}

func (p Permissions) Has(perms ...Permissions) bool {
	for _, perm := range perms {
		if p&perm == 0 {
			return false
		}
	}
	return true
}

func (p Permissions) Add(perms ...Permissions) Permissions {
	for _, perm := range perms {
		p |= perm
	}
	return p
}

func (p Permissions) Remove(perms ...Permissions) Permissions {
	for _, perm := range perms {
		p &^= perm
	}
	return p
}

func (p Permissions) String() string {
	if p == PermissionsNone {
		return "none"
	}
	var perms []string
	for perm, name := range AllPermissions {
		if p.Has(perm) {
			perms = append(perms, name)
		}
	}
	return strings.Join(perms, ",")
}

func (s *Server) Permissions(f []FilePermissions, filePath string, userInfo *UserInfo) Permissions {
	if s.cfg.Auth == nil || (s.cfg.Auth != nil && slices.Contains(userInfo.Groups, s.cfg.Auth.Groups.Admin)) {
		return PermissionsAll
	}

	parts := []string{"/"}
	var currentPart string
	for _, part := range strings.Split(filePath, "/") {
		if part == "" {
			continue
		}
		currentPart += "/" + part
		parts = append(parts, currentPart)
	}

	var rolePermissions Permissions
	loopPermissions(f, parts, func(perm FilePermissions) {
		if perm.ObjectType == ObjectTypeGroup && slices.Contains(userInfo.Groups, perm.Object) {
			rolePermissions = rolePermissions.Add(perm.Permissions)
		}
	})
	var userPermissions Permissions
	loopPermissions(f, parts, func(perm FilePermissions) {
		if perm.ObjectType == ObjectTypeUser && perm.Object == userInfo.Subject {
			userPermissions = userPermissions.Add(perm.Permissions)
		}
	})
	return rolePermissions.Add(userPermissions)
}

func loopPermissions(filePerms []FilePermissions, paths []string, f func(permissions FilePermissions)) Permissions {
	var permissions Permissions
	for _, path := range paths {
		for _, perm := range filePerms {
			if perm.Path != path {
				continue
			}
			f(perm)
		}
	}
	return permissions
}

func (s *Server) PutPermissions(w http.ResponseWriter, r *http.Request) {
	var perms []PermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&perms); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.db.DeleteAllFilePermissions(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, perm := range perms {
		if err := s.db.UpsertFilePermission(r.Context(), perm.Path, perm.Permissions, perm.ObjectType, perm.Object); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}