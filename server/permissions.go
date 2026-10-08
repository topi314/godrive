package server

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"path"
	"time"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/database/dbq"
	"github.com/topi314/godrive/server/storage"
)

func aclIdentity(info *UserInfo) *acl.Identity {
	if info == nil {
		return nil
	}
	return &acl.Identity{Subject: info.Subject, Groups: info.Groups}
}

func (s *Server) EffectivePermissions(ctx context.Context, filePath string, info *UserInfo, ownerID *string) (acl.Permissions, error) {
	if !s.cfg.AuthEnabled() {
		return acl.PermissionsAll, nil
	}
	if s.adminSudo(info) {
		return acl.PermissionsAll, nil
	}
	if info != nil && info.Subject != "" && info.Subject != "guest" {
		if ownerID != nil && *ownerID != "" && info.Subject == *ownerID {
			return acl.PermissionsAll, nil
		}
		if ownerID == nil {
			if file, err := s.store.Q.GetFile(ctx, acl.NormalizePath(filePath)); err == nil && file.UserID.Valid && file.UserID.String == info.Subject {
				return acl.PermissionsAll, nil
			}
		}
	}
	if info == nil && !s.cfg.Auth.Groups.Guest {
		return 0, nil
	}
	return s.aclPermsFor(ctx, filePath, aclIdentity(info))
}

func (s *Server) CanAnonymousRead(ctx context.Context, filePath string) (bool, error) {
	if s.cfg.AuthEnabled() && !s.cfg.Auth.Groups.Guest {
		return false, nil
	}
	perms, err := s.aclPermsFor(ctx, filePath, nil)
	if err != nil {
		return false, err
	}
	return perms.Has(acl.PermissionRead), nil
}

func (s *Server) aclPermsFor(ctx context.Context, filePath string, id *acl.Identity) (acl.Permissions, error) {
	paths := acl.AncestorPathsRootFirst(filePath)
	rows, err := s.store.ListACLByPaths(ctx, paths)
	if err != nil {
		return 0, err
	}
	return acl.CalculatePermissions(paths, rows, id), nil
}

// seedDefaultRootACL inserts a starter ACL on "/" when none exist yet:
// authenticated users (everyone) get full access; guests get read.
func (s *Server) seedDefaultRootACL(ctx context.Context) error {
	if !s.cfg.AuthEnabled() {
		return nil
	}
	rows, err := s.store.Q.ListAllACL(ctx)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		return nil
	}
	if _, err := s.store.Q.UpsertACL(ctx, dbq.UpsertACLParams{
		Path: "/", PrincipalType: acl.PrincipalEveryone, PrincipalID: acl.EveryoneID,
		Allow: int64(acl.PermissionsAll), Deny: 0,
	}); err != nil {
		return err
	}
	if _, err := s.store.Q.UpsertACL(ctx, dbq.UpsertACLParams{
		Path: "/", PrincipalType: acl.PrincipalGuest, PrincipalID: acl.GuestID,
		Allow: int64(acl.PermissionRead), Deny: 0,
	}); err != nil {
		return err
	}
	slog.Info("seeded default root ACL",
		slog.String("everyone", "all"),
		slog.String("guest", "read"),
	)
	return nil
}

// provisionUserHome creates the user's home folder (parents included) and grants
// them owner-like ACL on that path. Other principals' ACLs are left alone; the
// owner's full-access ACL is added when missing.
func (s *Server) shouldProvisionHome(ctx context.Context, home string) bool {
	home = acl.NormalizePath(home)
	if home == "/" {
		return false
	}
	file, err := s.store.Q.GetFile(ctx, home)
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	if err != nil {
		return false
	}
	return !storage.IsDirectory(file.ContentType)
}

func (s *Server) provisionUserHome(ctx context.Context, home string, info *UserInfo) error {
	if !s.cfg.AuthEnabled() || info == nil || s.isGuest(info) || info.Subject == "" {
		return nil
	}
	home = acl.NormalizePath(home)
	if home == "/" {
		return nil
	}
	if acl.IsReservedPath(home) {
		return errors.New("reserved path")
	}

	now := time.Now().UTC()
	var missing []string
	for p := home; p != "/"; p = path.Dir(p) {
		file, err := s.store.Q.GetFile(ctx, p)
		if err == nil {
			if p == home && !storage.IsDirectory(file.ContentType) {
				return errors.New("home is not a folder")
			}
			break
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		ok, err := s.pathExists(ctx, p)
		if err != nil {
			return err
		}
		if ok && p != home {
			break
		}
		missing = append(missing, p)
	}

	for i := len(missing) - 1; i >= 0; i-- {
		p := missing[i]
		if err := s.storage.Mkdir(ctx, p); err != nil {
			return err
		}
		owner := ""
		if p == home {
			owner = info.Subject
		}
		if _, err := s.store.Q.UpsertFile(ctx, dirFileParams(p, now, owner)); err != nil {
			return err
		}
	}

	file, err := s.store.Q.GetFile(ctx, home)
	if errors.Is(err, sql.ErrNoRows) {
		if err := s.storage.Mkdir(ctx, home); err != nil {
			return err
		}
		file, err = s.store.Q.UpsertFile(ctx, dirFileParams(home, now, info.Subject))
	}
	if err != nil {
		return err
	}
	if !storage.IsDirectory(file.ContentType) {
		return errors.New("home is not a folder")
	}
	if file.UserID.Valid && file.UserID.String != info.Subject {
		return nil
	}
	if !file.UserID.Valid {
		file, err = s.store.Q.UpsertFile(ctx, dbq.UpsertFileParams{
			Path: file.Path, Size: file.Size, ContentType: file.ContentType,
			Description: file.Description, UserID: database.NullString(&info.Subject),
			CreatedAt: file.CreatedAt, UpdatedAt: now,
		})
		if err != nil {
			return err
		}
	}

	existing, err := s.store.Q.ListACLByPath(ctx, home)
	if err != nil {
		return err
	}
	for _, row := range existing {
		if row.PrincipalType == acl.PrincipalUser && row.PrincipalID == info.Subject {
			return nil
		}
	}
	_, err = s.store.Q.UpsertACL(ctx, dbq.UpsertACLParams{
		Path:          home,
		PrincipalType: acl.PrincipalUser,
		PrincipalID:   info.Subject,
		Allow:         int64(acl.PermissionsAll),
		Deny:          0,
	})
	return err
}
