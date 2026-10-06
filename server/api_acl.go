package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database/dbq"
)

func aclJSON(row dbq.PathAcl) map[string]any {
	return map[string]any{
		"path":           row.Path,
		"principal_type": row.PrincipalType,
		"principal_id":   row.PrincipalID,
		"allow":          row.Allow,
		"deny":           row.Deny,
	}
}

func (s *Server) GetPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	p := acl.NormalizePath(r.URL.Query().Get("path"))
	info := GetUserInfo(r)
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(acl.PermissionRead) && !perms.Has(acl.PermissionUpdatePermissions)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
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
	s.writeJSON(w, map[string]any{
		"path":             p,
		"effective":        uint64(perms),
		"acl":              local,
		"inherited":        inherited,
		"available_groups": s.availableGroups(),
		"available_users":  availableUsers,
	}, http.StatusOK)
}

func (s *Server) PutPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	p := acl.NormalizePath(r.URL.Query().Get("path"))
	if acl.IsReservedPath(p) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	info := GetUserInfo(r)
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(acl.PermissionUpdatePermissions) && !s.isAdmin(info)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	var body struct {
		ACL []struct {
			PrincipalType string `json:"principal_type"`
			PrincipalID   string `json:"principal_id"`
			Allow         int64  `json:"allow"`
			Deny          int64  `json:"deny"`
		} `json:"acl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	_ = s.store.Q.DeleteACLForPath(r.Context(), p)
	for _, row := range body.ACL {
		pid := row.PrincipalID
		switch row.PrincipalType {
		case acl.PrincipalEveryone:
			pid = acl.EveryoneID
		case acl.PrincipalGuest:
			pid = acl.GuestID
		}
		if row.PrincipalType == acl.PrincipalGroup {
			if pid == "" || (s.cfg.Auth != nil && !s.cfg.Auth.Groups.IsAvailableGroup(pid)) {
				s.writeError(w, r, errors.New("unknown group"), http.StatusBadRequest)
				return
			}
		}
		if row.PrincipalType == acl.PrincipalUser {
			if pid == "" {
				s.writeError(w, r, errors.New("user rules need a user id"), http.StatusBadRequest)
				return
			}
			if _, err := s.store.Q.GetUser(r.Context(), pid); err != nil {
				s.writeError(w, r, errors.New("unknown user"), http.StatusBadRequest)
				return
			}
		}
		if row.PrincipalType == acl.PrincipalShare {
			if pid == "" {
				s.writeError(w, r, errors.New("share rules need a share id"), http.StatusBadRequest)
				return
			}
			if _, err := s.store.Q.GetShare(r.Context(), pid); err != nil {
				s.writeError(w, r, errors.New("unknown share"), http.StatusBadRequest)
				return
			}
		}
		_, err := s.store.Q.UpsertACL(r.Context(), dbq.UpsertACLParams{
			Path: p, PrincipalType: row.PrincipalType, PrincipalID: pid,
			Allow: row.Allow, Deny: row.Deny,
		})
		if err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
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
