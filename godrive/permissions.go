package godrive

import (
	"encoding/json"
	"golang.org/x/exp/slices"
	"net/http"
	"strings"
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
	if s.cfg.Auth != nil && slices.Contains(userInfo.Groups, s.cfg.Auth.Groups.Admin) {
		return PermissionsAll
	}
	var parts []string
	if filePath == "/" {
		parts = []string{"/"}
	} else {
		parts = strings.FieldsFunc(filePath, func(r rune) bool {
			return r == '/'
		})
	}

	var permissions Permissions
	filePart := "/"
	for _, part := range parts {
		var (
			perms    Permissions
			hasPerms bool
		)
		for _, perm := range f {
			if perm.Path != filePart {
				continue
			}
			if perm.ObjectType == ObjectTypeUser && perm.Object == userInfo.Subject {
				hasPerms = true
				perms = perms.Add(perm.Permissions)
			}
			if perm.ObjectType == ObjectTypeGroup && slices.Contains(userInfo.Groups, perm.Object) {
				hasPerms = true
				perms = perms.Add(perm.Permissions)
			}
		}
		if hasPerms {
			permissions = perms
		}
		if strings.HasSuffix(filePart, "/") {
			filePart += part
		} else {
			filePart += "/" + part
		}
	}
	return permissions
}

func (s *Server) PutPermissions(w http.ResponseWriter, r *http.Request) {
	userInfo := s.GetUserInfo(r)
	if !s.isAdmin(userInfo) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var perms []PermissionsRequest
	if err := json.NewDecoder(r.Body).Decode(&perms); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
