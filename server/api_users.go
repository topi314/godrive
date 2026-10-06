package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database/dbq"
)

func (s *Server) PatchMeAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	if info == nil || s.isGuest(info) || info.Subject == "" {
		s.writeError(w, r, errors.New("unauthorized"), http.StatusUnauthorized)
		return
	}
	if s.cfg.Auth == nil {
		s.writeError(w, r, errors.New("profile settings require authentication"), http.StatusNotImplemented)
		return
	}

	var body struct {
		Home *string `json:"home"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	if body.Home == nil {
		s.writeError(w, r, errors.New("home is required"), http.StatusBadRequest)
		return
	}
	home := acl.NormalizePath(*body.Home)
	user, err := s.store.Q.UpdateUserHome(r.Context(), dbq.UpdateUserHomeParams{
		ID:        info.Subject,
		Home:      home,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	if err := s.provisionUserHome(r.Context(), user.Home, s.userToInfo(user)); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	s.writeJSON(w, s.userPublicJSON(r.Context(), user), http.StatusOK)
}

func (s *Server) PatchUserAPI(w http.ResponseWriter, r *http.Request) {
	if !s.isAdmin(GetUserInfo(r)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		s.writeError(w, r, errors.New("missing user id"), http.StatusBadRequest)
		return
	}
	var body struct {
		Home *string `json:"home"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	if body.Home == nil {
		s.writeError(w, r, errors.New("home is required"), http.StatusBadRequest)
		return
	}
	home := acl.NormalizePath(*body.Home)
	user, err := s.store.Q.UpdateUserHome(r.Context(), dbq.UpdateUserHomeParams{
		ID:        id,
		Home:      home,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	if err := s.provisionUserHome(r.Context(), user.Home, s.userToInfo(user)); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	s.writeJSON(w, s.userPublicJSON(r.Context(), user), http.StatusOK)
}

func (s *Server) DeleteUserAPI(w http.ResponseWriter, r *http.Request) {
	admin := GetUserInfo(r)
	if !s.isAdmin(admin) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		s.writeError(w, r, errors.New("missing user id"), http.StatusBadRequest)
		return
	}
	if admin != nil && admin.Subject == id {
		s.writeError(w, r, errors.New("cannot delete your own account"), http.StatusBadRequest)
		return
	}
	_ = s.store.Q.DeleteSessionsForUser(r.Context(), id)
	_ = s.store.Q.DeleteAPITokensForUser(r.Context(), id)
	if err := s.store.Q.DeleteUser(r.Context(), id); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
