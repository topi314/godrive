package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/storage"
)

type aclRuleBody struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Allow         int64  `json:"allow"`
	Deny          int64  `json:"deny"`
}

type aclPrincipalBody struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
}

func aclJSON(row database.PathAcl) map[string]any {
	return map[string]any{
		"path":           row.Path,
		"principal_type": row.PrincipalType,
		"principal_id":   row.PrincipalID,
		"allow":          row.Allow,
		"deny":           row.Deny,
	}
}

func aclPath(r *http.Request) string {
	p := chi.URLParam(r, "*")
	if p == "" {
		return "/"
	}
	return acl.NormalizePath(p)
}

func (s *Server) canReadPermissions(ctx context.Context, p string, info *UserInfo) bool {
	perms, err := s.EffectivePermissions(ctx, p, info, nil)
	if err != nil {
		return false
	}
	if !s.cfg.AuthEnabled() {
		return true
	}
	return perms.Has(acl.PermissionRead) || perms.Has(acl.PermissionUpdatePermissions)
}

func (s *Server) canUpdatePermissions(ctx context.Context, p string, info *UserInfo) bool {
	perms, err := s.EffectivePermissions(ctx, p, info, nil)
	if err != nil {
		return false
	}
	if !s.cfg.AuthEnabled() {
		return true
	}
	return perms.Has(acl.PermissionUpdatePermissions) || s.adminSudo(info)
}

func normalizeACLPrincipalID(principalType, principalID string) (string, string, error) {
	pid := principalID
	switch principalType {
	case acl.PrincipalEveryone:
		pid = acl.EveryoneID
	case acl.PrincipalGuest:
		pid = acl.GuestID
	case acl.PrincipalGroup, acl.PrincipalUser, acl.PrincipalShare:
		if pid == "" {
			return "", "", errors.New(principalType + " rules need a principal id")
		}
	default:
		return "", "", errors.New("unknown principal type")
	}
	return principalType, pid, nil
}

func (s *Server) validateACLRule(ctx context.Context, row aclRuleBody) (database.UpsertACLParams, error) {
	pt, pid, err := normalizeACLPrincipalID(row.PrincipalType, row.PrincipalID)
	if err != nil {
		return database.UpsertACLParams{}, err
	}
	switch pt {
	case acl.PrincipalGroup:
		if s.cfg.AuthEnabled() && !s.cfg.Auth.Groups.IsAvailableGroup(pid) {
			return database.UpsertACLParams{}, errors.New("unknown group")
		}
	case acl.PrincipalUser:
		if _, err := s.store.Q.GetUser(ctx, pid); err != nil {
			return database.UpsertACLParams{}, errors.New("unknown user")
		}
	case acl.PrincipalShare:
		if _, err := s.store.Q.GetShare(ctx, pid); err != nil {
			return database.UpsertACLParams{}, errors.New("unknown share")
		}
	}
	return database.UpsertACLParams{
		PrincipalType: pt,
		PrincipalID:   pid,
		Allow:         row.Allow,
		Deny:          row.Deny,
	}, nil
}

func (s *Server) GetPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	p := aclPath(r)
	info := GetUserInfo(r)
	if !s.canReadPermissions(r.Context(), p, info) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	paths := acl.AncestorPathsRootFirst(p)
	all, err := s.store.Q.ListACLByPaths(r.Context(), paths)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	byPath := map[string][]map[string]any{}
	for _, row := range all {
		byPath[row.Path] = append(byPath[row.Path], aclJSON(row))
	}
	local := byPath[p]
	if local == nil {
		local = []map[string]any{}
	}
	inherited := make([]map[string]any, 0)
	for _, ancestor := range paths {
		if ancestor == p {
			continue
		}
		inherited = append(inherited, byPath[ancestor]...)
	}
	availableUsers := []map[string]any{}
	if users, err := s.store.Q.ListUsers(r.Context()); err == nil {
		availableUsers = make([]map[string]any, 0, len(users))
		for _, u := range users {
			availableUsers = append(availableUsers, map[string]any{
				"id":       u.ID,
				"username": u.Username,
				"email":    u.Email,
			})
		}
	}
	ownerID := ""
	ownerName := ""
	isDir := p == "/"
	if file, err := s.store.Q.GetFile(r.Context(), p); err == nil {
		if file.UserID.Valid {
			ownerID = file.UserID.String
			if user, err := s.store.Q.GetUser(r.Context(), ownerID); err == nil {
				ownerName = user.Username
			}
		}
		isDir = storage.IsDirectory(file.ContentType)
	} else if p != "/" {
		under, err := s.store.Q.ListFilesUnder(r.Context(), database.ListFilesUnderParams{
			Path: p, PathLike: acl.LikeUnder(p),
		})
		if err == nil {
			for _, row := range under {
				if row.Path != p {
					isDir = true
					break
				}
			}
		}
	}
	s.writeJSON(w, map[string]any{
		"path":             p,
		"owner_id":         ownerID,
		"owner":            ownerName,
		"is_dir":           isDir,
		"effective":        uint64(perms),
		"acl":              local,
		"inherited":        inherited,
		"available_groups": s.availableGroups(),
		"available_users":  availableUsers,
	}, http.StatusOK)
}

func (s *Server) PutPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	p := aclPath(r)
	if acl.IsReservedPath(p) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	info := GetUserInfo(r)
	if !s.canUpdatePermissions(r.Context(), p, info) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	var body struct {
		ACL []aclRuleBody `json:"acl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	rules := make([]database.UpsertACLParams, 0, len(body.ACL))
	for _, row := range body.ACL {
		rule, err := s.validateACLRule(r.Context(), row)
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
		rule.Path = p
		rules = append(rules, rule)
	}

	if err := s.store.WithTx(r.Context(), func(q database.Querier) error {
		if err := q.DeleteACLForPath(r.Context(), p); err != nil {
			return err
		}
		for _, rule := range rules {
			if _, err := q.UpsertACL(r.Context(), rule); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) PatchPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	p := aclPath(r)
	if acl.IsReservedPath(p) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	info := GetUserInfo(r)
	if !s.canUpdatePermissions(r.Context(), p, info) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	var body struct {
		Upsert []aclRuleBody      `json:"upsert"`
		Remove []aclPrincipalBody `json:"remove"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	if len(body.Upsert) == 0 && len(body.Remove) == 0 {
		s.writeError(w, r, errors.New("upsert or remove required"), http.StatusBadRequest)
		return
	}

	removes := make([]database.DeleteACLParams, 0, len(body.Remove))
	for _, row := range body.Remove {
		pt, pid, err := normalizeACLPrincipalID(row.PrincipalType, row.PrincipalID)
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
		removes = append(removes, database.DeleteACLParams{
			Path: p, PrincipalType: pt, PrincipalID: pid,
		})
	}
	upserts := make([]database.UpsertACLParams, 0, len(body.Upsert))
	for _, row := range body.Upsert {
		rule, err := s.validateACLRule(r.Context(), row)
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
		rule.Path = p
		upserts = append(upserts, rule)
	}

	if err := s.store.WithTx(r.Context(), func(q database.Querier) error {
		for _, del := range removes {
			if err := q.DeleteACL(r.Context(), del); err != nil {
				return err
			}
		}
		for _, rule := range upserts {
			if _, err := q.UpsertACL(r.Context(), rule); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) ListAllPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(GetUserInfo(r)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	rows, err := s.store.Q.ListAllACL(r.Context())
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, rows, http.StatusOK)
}

func (s *Server) ListUsersAPI(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(GetUserInfo(r)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	users, err := s.store.Q.ListUsers(r.Context())
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(users))
	for _, u := range users {
		out = append(out, s.userPublicJSON(r.Context(), u))
	}
	s.writeJSON(w, out, http.StatusOK)
}
