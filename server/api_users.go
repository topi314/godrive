package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
)

func (s *Server) PatchMeAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	if info == nil || s.isGuest(info) || info.Subject == "" {
		s.writeError(w, r, errors.New("unauthorized"), http.StatusUnauthorized)
		return
	}
	if !s.cfg.AuthEnabled() {
		s.writeError(w, r, errors.New("profile settings require authentication"), http.StatusNotImplemented)
		return
	}
	var body struct {
		Home *string `json:"home"`
		Sudo *bool   `json:"sudo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	if body.Home == nil && body.Sudo == nil {
		s.writeError(w, r, errors.New("home or sudo required"), http.StatusBadRequest)
		return
	}
	if body.Sudo != nil {
		if !s.isAdmin(info) {
			s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
			return
		}
		s.setSudoCookie(w, *body.Sudo)
		info.Sudo = *body.Sudo
	}
	if body.Home != nil {
		user, err := s.updateUserHome(r.Context(), info.Subject, *body.Home)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, errBadHome) {
				status = http.StatusBadRequest
			}
			s.writeError(w, r, err, status)
			return
		}
		info.Home = user.Home
	}
	s.writeJSON(w, s.meResponse(r, info), http.StatusOK)
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
	user, err := s.updateUserHome(r.Context(), id, *body.Home)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errBadHome) {
			status = http.StatusBadRequest
		}
		s.writeError(w, r, err, status)
		return
	}
	s.writeJSON(w, s.userPublicJSON(r.Context(), user), http.StatusOK)
}

var errBadHome = errors.New("invalid home")

func (s *Server) updateUserHome(ctx context.Context, userID, homeRaw string) (database.User, error) {
	home := acl.NormalizePath(homeRaw)
	user, err := s.store.Q.UpdateUserHome(ctx, database.UpdateUserHomeParams{
		ID:        userID,
		Home:      home,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		return database.User{}, err
	}
	if err := s.provisionUserHome(ctx, user.Home, s.userToInfo(user)); err != nil {
		return database.User{}, errors.Join(errBadHome, err)
	}
	return user, nil
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
