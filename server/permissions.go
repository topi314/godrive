package server

import (
	"context"
	"path"
	"strings"
)

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
	PrincipalEveryone = "everyone"
	EveryoneID        = "*"
)

type ACLRow struct {
	Path          string
	PrincipalType string
	PrincipalID   string
	Allow         Permissions
	Deny          Permissions
}

// NormalizePath returns an absolute path without trailing slash (except root "/").
func NormalizePath(p string) string {
	if p == "" {
		return "/"
	}
	p = path.Clean("/" + strings.TrimPrefix(p, "/"))
	if p == "." {
		return "/"
	}
	return p
}

// AncestorPaths returns [path, parent, ..., "/"] from leaf to root.
func AncestorPaths(filePath string) []string {
	filePath = NormalizePath(filePath)
	var paths []string
	for {
		paths = append(paths, filePath)
		if filePath == "/" {
			break
		}
		parent := path.Dir(filePath)
		if parent == filePath {
			break
		}
		filePath = parent
	}
	return paths
}

// AncestorPathsRootFirst returns ["/", ..., parent, path].
func AncestorPathsRootFirst(filePath string) []string {
	paths := AncestorPaths(filePath)
	for i, j := 0, len(paths)-1; i < j; i, j = i+1, j-1 {
		paths[i], paths[j] = paths[j], paths[i]
	}
	return paths
}

func LikeUnder(prefix string) string {
	prefix = NormalizePath(prefix)
	if prefix == "/" {
		return "/%"
	}
	return prefix + "/%"
}

type ACLStore interface {
	ListACLByPaths(ctx context.Context, paths []string) ([]ACLRow, error)
}

func CalculatePermissions(pathsRootFirst []string, rows []ACLRow, info *UserInfo) Permissions {
	byPath := map[string][]ACLRow{}
	for _, row := range rows {
		byPath[row.Path] = append(byPath[row.Path], row)
	}

	var allow, deny Permissions
	for _, p := range pathsRootFirst {
		for _, row := range byPath[p] {
			switch row.PrincipalType {
			case PrincipalEveryone:
				allow = allow.Add(row.Allow)
				deny = deny.Add(row.Deny)
			case PrincipalGroup:
				if info != nil && containsString(info.Groups, row.PrincipalID) {
					allow = allow.Add(row.Allow)
					deny = deny.Add(row.Deny)
				}
			case PrincipalUser:
				if info != nil && info.Subject == row.PrincipalID {
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

func (s *Server) EffectivePermissions(ctx context.Context, filePath string, info *UserInfo, ownerID *string) (Permissions, error) {
	if s.cfg.Auth == nil {
		return PermissionsAll, nil
	}
	if info != nil && s.isAdmin(info) {
		return PermissionsAll, nil
	}
	if ownerID != nil && info != nil && *ownerID != "" && info.Subject == *ownerID {
		return PermissionsAll, nil
	}

	paths := AncestorPathsRootFirst(filePath)
	rows, err := s.store.ListACLByPaths(ctx, paths)
	if err != nil {
		return 0, err
	}
	return CalculatePermissions(paths, rows, info), nil
}

func (s *Server) CanAnonymousRead(ctx context.Context, filePath string) (bool, error) {
	paths := AncestorPathsRootFirst(filePath)
	rows, err := s.store.ListACLByPaths(ctx, paths)
	if err != nil {
		return false, err
	}
	perms := CalculatePermissions(paths, rows, nil)
	return perms.Has(PermissionRead), nil
}
