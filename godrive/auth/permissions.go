package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/topi314/godrive/godrive/database"
)

const (
	PermissionRead Permissions = 1 << iota
	PermissionCreate
	PermissionUpdate
	PermissionDelete
	PermissionUpdatePermissions
	PermissionShare

	PermissionsAll = PermissionRead | PermissionCreate | PermissionUpdate | PermissionDelete | PermissionUpdatePermissions | PermissionShare
)

var AllPermissions = map[Permissions]string{
	PermissionRead:              "read",
	PermissionCreate:            "create",
	PermissionUpdate:            "update",
	PermissionDelete:            "delete",
	PermissionUpdatePermissions: "update_permissions",
	PermissionShare:             "share",
}

type Permissions int

func (p Permissions) String() string {
	perms := p.Slice()
	if len(perms) == 0 {
		return "none"
	}
	return strings.Join(perms, ", ")
}

func (p Permissions) Map() map[string]bool {
	perms := make(map[string]bool)
	for perm, name := range AllPermissions {
		perms[name] = p.Has(perm)
	}
	return perms
}

func (p Permissions) Slice() []string {
	var perms []string
	for perm, name := range AllPermissions {
		if p.Has(perm) {
			perms = append(perms, name)
		}
	}
	return perms
}

func (p Permissions) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Slice())
}

func (p *Permissions) UnmarshalJSON(data []byte) error {
	var perms []string
	if err := json.Unmarshal(data, &perms); err != nil {
		return err
	}
	for _, perm := range perms {
		for permVal, name := range AllPermissions {
			if perm == name {
				*p = p.Add(permVal)
			}
		}
	}
	return nil
}

func (p Permissions) HasAny(perms ...Permissions) bool {
	for _, perm := range perms {
		if p&perm != 0 {
			return true
		}
	}
	return false
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
		p &= ^perm
	}
	return p
}

const (
	ObjectTypeUser ObjectType = iota
	ObjectTypeGroup
	ObjectTypeEveryone
)

var allObjectTypes = map[ObjectType]string{
	ObjectTypeUser:     "user",
	ObjectTypeGroup:    "group",
	ObjectTypeEveryone: "everyone",
}

type ObjectType int

func (o ObjectType) String() string {
	return allObjectTypes[o]
}

func (o ObjectType) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.String())
}

func (o *ObjectType) UnmarshalJSON(data []byte) error {
	var objType string
	if err := json.Unmarshal(data, &objType); err != nil {
		return err
	}
	for objTypeVal, name := range allObjectTypes {
		if objType == name {
			*o = objTypeVal
			return nil
		}
	}
	return fmt.Errorf("unknown object type '%s'", objType)
}

func (a *Auth) GetFilesPermissions(ctx context.Context, filePaths []string, info *UserInfo) (map[string]Permissions, error) {
	var allPaths []string
	paths := make([][]string, len(filePaths))
	for i, filePath := range filePaths {
		currentPath := filePath
		for {
			if !slices.Contains(allPaths, currentPath) {
				allPaths = append(allPaths, currentPath)
			}
			paths[i] = append(paths[i], currentPath)
			if currentPath == "/" {
				break
			}
			currentPath = path.Dir(currentPath)
		}
	}

	perms, err := a.db.GetPermissions(ctx, allPaths)
	if err != nil {
		return nil, fmt.Errorf("error getting path permissions: %w", err)
	}

	finalPerms := make(map[string]Permissions, len(filePaths))
	for i, filePath := range filePaths {
		finalPerms[filePath] = a.calculatePermissions(paths[i], perms, info)
	}

	return finalPerms, nil
}

func (a *Auth) GetFilePermissions(ctx context.Context, filePath string, info *UserInfo) (Permissions, error) {
	if a.cfg == nil || a.IsAdmin(info.Groups) {
		return PermissionsAll, nil
	}

	var paths []string
	currentPath := filePath
	for {
		paths = append(paths, currentPath)
		if currentPath == "/" {
			break
		}
		currentPath = path.Dir(currentPath)
	}

	perms, err := a.db.GetPermissions(ctx, paths)
	if err != nil {
		return 0, fmt.Errorf("error getting path permissions: %w", err)
	}

	slices.Reverse(paths)
	return a.calculatePermissions(paths, perms, info), nil
}

func (a *Auth) calculatePermissions(paths []string, perms []database.Permissions, info *UserInfo) Permissions {
	var (
		allow Permissions
		deny  Permissions
	)
	for _, part := range paths {
		for _, perm := range perms {
			if perm.Path == part && perm.ObjectType == int(ObjectTypeEveryone) {
				allow = allow.Add(Permissions(perm.Allow))
				deny = deny.Add(Permissions(perm.Deny))
			}
		}

		for _, perm := range perms {
			if perm.Path == part && perm.ObjectType == int(ObjectTypeGroup) && slices.Contains(info.Groups, perm.Object) {
				allow = allow.Add(Permissions(perm.Allow))
				deny = deny.Add(Permissions(perm.Deny))
			}
		}

		for _, perm := range perms {
			if perm.Path == part && perm.ObjectType == int(ObjectTypeUser) && perm.Object == info.Subject {
				allow = allow.Add(Permissions(perm.Allow))
				deny = deny.Add(Permissions(perm.Deny))
			}
		}
	}
	return allow.Remove(deny)
}
