package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/topi314/godrive/server/database/dbsqlc"
)

func (s *Server) GetPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	p := NormalizePath(r.URL.Query().Get("path"))
	info := GetUserInfo(r)
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(PermissionRead) && !perms.Has(PermissionUpdatePermissions)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	rows, err := s.store.Q.ListACLByPath(r.Context(), p)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, len(rows))
	for i, row := range rows {
		out[i] = map[string]any{
			"path":           row.Path,
			"principal_type": row.PrincipalType,
			"principal_id":   row.PrincipalID,
			"allow":          row.Allow,
			"deny":           row.Deny,
		}
	}
	s.writeJSON(w, map[string]any{"path": p, "effective": uint64(perms), "acl": out}, http.StatusOK)
}

func (s *Server) PutPermissionsAPI(w http.ResponseWriter, r *http.Request) {
	p := NormalizePath(r.URL.Query().Get("path"))
	if IsReservedPath(p) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	info := GetUserInfo(r)
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(PermissionUpdatePermissions) && !s.isAdmin(info)) {
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
		if row.PrincipalType == PrincipalEveryone {
			pid = EveryoneID
		}
		_, err := s.store.Q.UpsertACL(r.Context(), dbsqlc.UpsertACLParams{
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
	s.writeJSON(w, users, http.StatusOK)
}
