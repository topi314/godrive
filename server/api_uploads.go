package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/storage"
)

type uploadPreflightBody struct {
	Dir         string `json:"dir"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	Replace     bool   `json:"replace"`
	ShareID     string `json:"share_id"`
}

func (s *Server) UploadConfigAPI(w http.ResponseWriter, r *http.Request) {
	s.writeJSON(w, map[string]any{
		"max_size":     s.cfg.Upload.MaxSize.Bytes,
		"chunk_size":   s.cfg.Upload.ChunkSize.Bytes,
		"session_ttl":  s.cfg.Upload.SessionTTL.Duration.String(),
		"max_parallel": s.cfg.Upload.MaxParallel,
	}, http.StatusOK)
}

func (s *Server) uploadIdentity(r *http.Request, shareID string) (*UserInfo, *acl.Identity, error) {
	info := GetUserInfo(r)
	if shareID != "" {
		share, err := s.store.Q.GetShare(r.Context(), shareID)
		if err != nil {
			return nil, nil, err
		}
		if share.ExpiresAt.Valid && !share.ExpiresAt.Time.After(time.Now().UTC()) {
			return nil, nil, errors.New("share expired")
		}
		// Share-link evaluation uses ShareID only in CalculatePermissions.
		return info, &acl.Identity{ShareID: shareID}, nil
	}
	if info == nil || s.isGuest(info) {
		return nil, nil, errors.New("unauthorized")
	}
	return info, aclIdentity(info), nil
}

func (s *Server) effectiveUploadPerms(ctx context.Context, filePath string, info *UserInfo, ident *acl.Identity, ownerID *string) (acl.Permissions, error) {
	if ident != nil && ident.ShareID != "" {
		return s.aclPermsFor(ctx, filePath, ident)
	}
	return s.EffectivePermissions(ctx, filePath, info, ownerID)
}

func (s *Server) validateUploadTarget(ctx context.Context, baseDir, name string, size int64, replace bool, info *UserInfo, ident *acl.Identity, shareRoot string) (string, []string) {
	var errs []string
	if size < 0 {
		errs = append(errs, "invalid size")
	}
	if size > s.cfg.Upload.MaxSize.Bytes {
		errs = append(errs, "file too large")
	}
	target, err := resolveUploadTarget(baseDir, name)
	if err != nil {
		errs = append(errs, err.Error())
		return "", errs
	}
	if shareRoot != "" && !acl.IsSelfOrUnder(target, shareRoot) {
		errs = append(errs, "path outside share")
		return target, errs
	}
	parent := path.Dir(target)
	perms, err := s.effectiveUploadPerms(ctx, parent, info, ident, nil)
	if err != nil || !perms.Has(acl.PermissionCreate) {
		errs = append(errs, "create forbidden")
	}
	existing, err := s.store.Q.GetFile(ctx, target)
	if err == nil {
		if !replace {
			errs = append(errs, "file exists")
		} else {
			owner := ""
			if existing.UserID.Valid {
				owner = existing.UserID.String
			}
			up, uerr := s.effectiveUploadPerms(ctx, target, info, ident, nullableStr(owner))
			if uerr != nil || !up.Has(acl.PermissionUpdate) {
				errs = append(errs, "replace forbidden")
			}
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		errs = append(errs, "lookup failed")
	}
	return target, errs
}

func (s *Server) enforceUploadParallel(ctx context.Context, userID, shareID string, now time.Time) error {
	limit := int64(s.cfg.Upload.MaxParallel)
	if limit <= 0 {
		return nil
	}
	var (
		n   int64
		err error
	)
	if shareID != "" {
		n, err = s.store.Q.CountActiveUploadSessionsByShare(ctx, database.CountActiveUploadSessionsByShareParams{
			ShareID: database.NullString(&shareID),
			Now:     now,
		})
	} else if userID != "" {
		n, err = s.store.Q.CountActiveUploadSessionsByUser(ctx, database.CountActiveUploadSessionsByUserParams{
			UserID: userID,
			Now:    now,
		})
	} else {
		return nil
	}
	if err != nil {
		return err
	}
	if n >= limit {
		return errors.New("too many concurrent uploads")
	}
	return nil
}

func (s *Server) PreflightUploadAPI(w http.ResponseWriter, r *http.Request) {
	var body uploadPreflightBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	info, ident, err := s.uploadIdentity(r, body.ShareID)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, sql.ErrNoRows) || err.Error() == "share expired" {
			status = http.StatusForbidden
		}
		s.writeError(w, r, err, status)
		return
	}
	shareRoot := ""
	base := body.Dir
	if body.ShareID != "" {
		share, err := s.store.Q.GetShare(r.Context(), body.ShareID)
		if err != nil {
			s.writeError(w, r, err, http.StatusForbidden)
			return
		}
		shareRoot = share.Path
		base = resolveShareUploadDir(share, body.Dir)
	}
	target, errs := s.validateUploadTarget(r.Context(), base, body.Name, body.Size, body.Replace, info, ident, shareRoot)
	if len(errs) > 0 {
		s.writeJSON(w, map[string]any{"ok": false, "errors": errs, "path": target}, http.StatusOK)
		return
	}
	s.writeJSON(w, map[string]any{"ok": true, "path": target}, http.StatusOK)
}

type s3PartJSON struct {
	PartNumber int32  `json:"part_number"`
	ETag       string `json:"etag"`
}

func (s *Server) CreateUploadSessionAPI(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Dir         string `json:"dir"`
		Name        string `json:"name"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
		Description string `json:"description"`
		Replace     bool   `json:"replace"`
		ShareID     string `json:"share_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	info, ident, err := s.uploadIdentity(r, body.ShareID)
	if err != nil {
		s.writeError(w, r, err, http.StatusUnauthorized)
		return
	}
	shareRoot := ""
	base := body.Dir
	ownerSubject := ""
	if body.ShareID != "" {
		share, err := s.store.Q.GetShare(r.Context(), body.ShareID)
		if err != nil {
			s.writeError(w, r, err, http.StatusForbidden)
			return
		}
		shareRoot = share.Path
		ownerSubject = share.UserID
		base = resolveShareUploadDir(share, body.Dir)
	} else if info != nil {
		ownerSubject = info.Subject
	}
	target, errs := s.validateUploadTarget(r.Context(), base, body.Name, body.Size, body.Replace, info, ident, shareRoot)
	if len(errs) > 0 {
		s.writeJSON(w, map[string]any{"ok": false, "errors": errs}, http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	if err := s.enforceUploadParallel(r.Context(), ownerSubject, body.ShareID, now); err != nil {
		s.writeError(w, r, err, http.StatusTooManyRequests)
		return
	}
	id := randomID(24)
	tempKey := id
	uploadID, err := s.storage.CreateUpload(r.Context(), tempKey, body.Size)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	ct := body.ContentType
	if ct == "" {
		ct = sniffContentType(path.Base(target), "")
	}
	params := database.CreateUploadSessionParams{
		ID:          id,
		UserID:      ownerSubject,
		ShareID:     database.NullString(nullableStr(body.ShareID)),
		Path:        target,
		Size:        body.Size,
		ContentType: ct,
		Description: body.Description,
		UploadOffset: 0,
		ReplaceFile: 0,
		TempKey:     tempKey,
		S3UploadID:  database.NullString(nullableStr(uploadID)),
		S3Parts:     "[]",
		ExpiresAt:   now.Add(s.cfg.Upload.SessionTTL.Duration),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if body.Replace {
		params.ReplaceFile = 1
	}
	sess, err := s.store.Q.CreateUploadSession(r.Context(), params)
	if err != nil {
		_ = s.storage.AbortUpload(r.Context(), tempKey, uploadID)
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{
		"id":         sess.ID,
		"path":       sess.Path,
		"size":       sess.Size,
		"upload_offset": sess.UploadOffset,
		"chunk_size":    s.cfg.Upload.ChunkSize.Bytes,
		"expires_at": sess.ExpiresAt.UTC(),
	}, http.StatusCreated)
}

func (s *Server) GetUploadSessionAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := s.store.Q.GetUploadSession(r.Context(), id)
	if err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	if !s.canAccessUploadSession(r, sess) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	s.writeJSON(w, map[string]any{
		"id":     sess.ID,
		"path":   sess.Path,
		"size":   sess.Size,
		"upload_offset": sess.UploadOffset,
		"expires_at":    sess.ExpiresAt.UTC(),
	}, http.StatusOK)
}

func (s *Server) canAccessUploadSession(r *http.Request, sess database.UploadSession) bool {
	if sess.ExpiresAt.Before(time.Now().UTC()) {
		return false
	}
	info := GetUserInfo(r)
	if sess.ShareID.Valid && sess.ShareID.String != "" {
		// Anyone with the share link may continue the upload they started via that share.
		return true
	}
	return info != nil && !s.isGuest(info) && info.Subject == sess.UserID
}

func (s *Server) PatchUploadSessionAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := s.store.Q.GetUploadSession(r.Context(), id)
	if err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	if !s.canAccessUploadSession(r, sess) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	offsetHdr := r.Header.Get("Upload-Offset")
	off, err := strconv.ParseInt(offsetHdr, 10, 64)
	if err != nil {
		s.writeError(w, r, errors.New("missing Upload-Offset"), http.StatusBadRequest)
		return
	}
	if off != sess.UploadOffset {
		s.writeError(w, r, errors.New("upload_offset mismatch"), http.StatusConflict)
		return
	}
	n := r.ContentLength
	if n < 0 {
		s.writeError(w, r, errors.New("content-length required"), http.StatusBadRequest)
		return
	}
	if n > s.cfg.Upload.ChunkSize.Bytes {
		s.writeError(w, r, errors.New("chunk too large"), http.StatusRequestEntityTooLarge)
		return
	}
	if sess.UploadOffset+n > sess.Size {
		s.writeError(w, r, errors.New("chunk exceeds size"), http.StatusBadRequest)
		return
	}
	partNum := int32(0)
	var parts []s3PartJSON
	_ = json.Unmarshal([]byte(sess.S3Parts), &parts)
	uploadID := ""
	if sess.S3UploadID.Valid {
		uploadID = sess.S3UploadID.String
		partNum = int32(len(parts) + 1)
	}
	etag, err := s.storage.WriteUpload(r.Context(), sess.TempKey, sess.UploadOffset, r.Body, n, uploadID, partNum)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	if etag != "" {
		parts = append(parts, s3PartJSON{PartNumber: partNum, ETag: etag})
	}
	partsJSON, _ := json.Marshal(parts)
	now := time.Now().UTC()
	updated, err := s.store.Q.UpdateUploadSessionOffset(r.Context(), database.UpdateUploadSessionOffsetParams{
		ID:           sess.ID,
		UploadOffset: sess.UploadOffset + n,
		S3Parts:      string(partsJSON),
		UpdatedAt:    now,
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Upload-Offset", strconv.FormatInt(updated.UploadOffset, 10))
	s.writeJSON(w, map[string]any{"upload_offset": updated.UploadOffset, "size": updated.Size}, http.StatusOK)
}

func (s *Server) CompleteUploadSessionAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := s.store.Q.GetUploadSession(r.Context(), id)
	if err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	if !s.canAccessUploadSession(r, sess) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	if sess.UploadOffset != sess.Size {
		s.writeError(w, r, errors.New("incomplete upload"), http.StatusBadRequest)
		return
	}
	var body struct {
		Description string `json:"description"`
		ACL         []struct {
			PrincipalType string `json:"principal_type"`
			PrincipalID   string `json:"principal_id"`
			Allow         int64  `json:"allow"`
			Deny          int64  `json:"deny"`
		} `json:"acl"`
	}
	_ = decodeJSONOptional(r, &body)
	desc := sess.Description
	if body.Description != "" {
		desc = body.Description
	}
	var parts []storage.CompletedPart
	var rawParts []s3PartJSON
	_ = json.Unmarshal([]byte(sess.S3Parts), &rawParts)
	for _, p := range rawParts {
		parts = append(parts, storage.CompletedPart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	uploadID := ""
	if sess.S3UploadID.Valid {
		uploadID = sess.S3UploadID.String
	}
	if err := s.ensureDir(r.Context(), path.Dir(sess.Path), &UserInfo{Subject: sess.UserID}); err != nil {
		if sess.ShareID.Valid && sess.ShareID.String != "" {
			// Share Create was already validated; create intermediate folders as the share owner.
			if err2 := s.ensureDirUnchecked(r.Context(), path.Dir(sess.Path), sess.UserID); err2 != nil {
				s.writeError(w, r, err2, http.StatusInternalServerError)
				return
			}
		} else if !errors.Is(err, errForbidden) {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		} else {
			s.writeError(w, r, err, http.StatusForbidden)
			return
		}
	}
	if err := s.storage.CommitUpload(r.Context(), sess.TempKey, sess.Path, sess.ContentType, uploadID, parts); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	params := database.UpsertFileParams{
		Path: sess.Path, Size: sess.Size, ContentType: sess.ContentType, Description: desc,
		CreatedAt: now, UpdatedAt: now,
	}
	if sess.UserID != "" && sess.UserID != "guest" {
		params.UserID = database.NullString(&sess.UserID)
	}
	if _, err := s.store.Q.UpsertFile(r.Context(), params); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	info := GetUserInfo(r)
	if len(body.ACL) > 0 && info != nil && !s.isGuest(info) {
		perms, _ := s.EffectivePermissions(r.Context(), sess.Path, info, nullableStr(sess.UserID))
		if perms.Has(acl.PermissionUpdatePermissions) || s.adminSudo(info) || info.Subject == sess.UserID {
			_ = s.store.Q.DeleteACLForPath(r.Context(), sess.Path)
			for _, row := range body.ACL {
				_, _ = s.store.Q.UpsertACL(r.Context(), database.UpsertACLParams{
					Path: sess.Path, PrincipalType: row.PrincipalType, PrincipalID: row.PrincipalID,
					Allow: row.Allow, Deny: row.Deny,
				})
			}
		}
	}
	_ = s.store.Q.DeleteUploadSession(r.Context(), sess.ID)
	s.writeJSON(w, map[string]any{"path": sess.Path}, http.StatusOK)
}

func (s *Server) AbortUploadSessionAPI(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := s.store.Q.GetUploadSession(r.Context(), id)
	if err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	if !s.canAccessUploadSession(r, sess) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	uploadID := ""
	if sess.S3UploadID.Valid {
		uploadID = sess.S3UploadID.String
	}
	_ = s.storage.AbortUpload(r.Context(), sess.TempKey, uploadID)
	_ = s.store.Q.DeleteUploadSession(r.Context(), sess.ID)
	w.WriteHeader(http.StatusNoContent)
}
