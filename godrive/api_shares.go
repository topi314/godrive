package godrive

import (
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/godrive/db"
)

func (s *Server) CreateShareAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	var body struct {
		Path      string     `json:"path"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	p := NormalizePath(body.Path)
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(PermissionShare)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	id := s.newShareID()
	now := time.Now().UTC()
	share, err := s.store.Q.CreateShare(r.Context(), db.CreateShareParams{
		ID: id, Path: p, UserID: info.Subject, CreatedAt: now, ExpiresAt: nullTime(body.ExpiresAt),
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{
		"id": share.ID, "path": share.Path, "url": "/s/" + share.ID,
		"created_at": share.CreatedAt, "expires_at": share.ExpiresAt,
	}, http.StatusCreated)
}

func (s *Server) ListSharesAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	shares, err := s.store.Q.ListSharesByUser(r.Context(), info.Subject)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, shares, http.StatusOK)
}

func (s *Server) DeleteShareAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	id := chi.URLParam(r, "id")
	share, err := s.store.Q.GetShare(r.Context(), id)
	if err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	if share.UserID != info.Subject && !s.isAdmin(info) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	_ = s.store.Q.DeleteShare(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resolveShare(r *http.Request) (db.Share, string, error) {
	id := chi.URLParam(r, "id")
	share, err := s.store.Q.GetShare(r.Context(), id)
	if err != nil {
		return share, "", err
	}
	if share.ExpiresAt.Valid && time.Now().After(share.ExpiresAt.Time) {
		return share, "", errors.New("expired")
	}
	rest := chi.URLParam(r, "*")
	target := share.Path
	if rest != "" {
		target = NormalizePath(path.Join(share.Path, rest))
	}
	if target != share.Path && !strings.HasPrefix(target, share.Path+"/") {
		return share, "", errors.New("forbidden")
	}
	return share, target, nil
}

func (s *Server) GetSharePage(w http.ResponseWriter, r *http.Request) {
	share, target, err := s.resolveShare(r)
	if err != nil {
		if isBot(r) {
			s.writeOG(w, r, ogPrivate())
			return
		}
		s.writeError(w, r, err, http.StatusNotFound)
		return
	}
	_ = share

	if isBot(r) {
		if file, err := s.store.Q.GetFile(r.Context(), target); err == nil {
			s.writeOG(w, r, ogPublic(file.Path, file.Description, file.ContentType, file.Size))
		} else {
			s.writeOG(w, r, ogPublic(target, "", "", 0))
		}
		return
	}

	if wantsJSON(r) || r.URL.Query().Get("dl") != "" {
		if file, err := s.store.Q.GetFile(r.Context(), target); err == nil {
			if r.URL.Query().Get("dl") != "" || !wantsHTML(r) {
				s.streamFile(w, r, target, file.ContentType, r.URL.Query().Get("dl") != "")
				return
			}
		}
		entries, err := s.listDir(r.Context(), target, nil)
		if err != nil {
			// list under share root without ACL — share grants read
			rows, err := s.store.Q.ListFilesUnder(r.Context(), db.ListFilesUnderParams{
				Path: target, PathLike: LikeUnder(target),
			})
			if err != nil {
				s.writeError(w, r, err, http.StatusInternalServerError)
				return
			}
			out := make([]FileEntry, 0, len(rows))
			for _, row := range rows {
				owner := ""
				if row.UserID.Valid {
					owner = row.UserID.String
				}
				out = append(out, fileEntryFromRow(row, owner, PermissionRead))
			}
			s.writeJSON(w, map[string]any{"path": target, "share_id": share.ID, "files": out}, http.StatusOK)
			return
		}
		s.writeJSON(w, map[string]any{"path": target, "share_id": share.ID, "files": entries}, http.StatusOK)
		return
	}
	s.serveSPA(w, r)
}

func (s *Server) GetSharePreview(w http.ResponseWriter, r *http.Request) {
	_, target, err := s.resolveShare(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.serveImagePreview(w, r, target)
}
