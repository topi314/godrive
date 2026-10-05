package godrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/godrive/db"
)

type FileEntry struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	IsDir       bool   `json:"is_dir"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	Description string `json:"description,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	Date        string `json:"date"`
	Permissions uint64 `json:"permissions"`
}

func (s *Server) filePathFromRequest(r *http.Request) string {
	p := chi.URLParam(r, "*")
	if strings.HasPrefix(r.URL.Path, "/api/files") {
		p = strings.TrimPrefix(r.URL.Path, "/api/files")
	}
	if p == "" {
		p = r.URL.Path
	}
	return NormalizePath(p)
}

func (s *Server) ListFilesAPI(w http.ResponseWriter, r *http.Request) {
	dir := NormalizePath(r.URL.Query().Get("path"))
	s.syncPrefix(r.Context(), dir)
	entries, err := s.listDir(r.Context(), dir, GetUserInfo(r))
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "forbidden" {
			status = http.StatusForbidden
		}
		s.writeError(w, r, err, status)
		return
	}
	s.writeJSON(w, map[string]any{"path": dir, "files": entries}, http.StatusOK)
}

func (s *Server) listDir(ctx context.Context, dir string, info *UserInfo) ([]FileEntry, error) {
	dir = NormalizePath(dir)
	if s.cfg.Auth != nil {
		perms, err := s.EffectivePermissions(ctx, dir, info, nil)
		if err != nil {
			return nil, err
		}
		if !perms.Has(PermissionRead) {
			anon, _ := s.CanAnonymousRead(ctx, dir)
			if !anon {
				return nil, errors.New("forbidden")
			}
		}
	}

	rows, err := s.store.Q.ListFilesUnder(ctx, db.ListFilesUnderParams{
		Path:     dir,
		PathLike: LikeUnder(dir),
	})
	if err != nil {
		return nil, err
	}

	type agg struct{ entry FileEntry }
	dirs := map[string]*agg{}
	var files []FileEntry

	for _, row := range rows {
		owner := ""
		if row.UserID.Valid {
			owner = row.UserID.String
		}
		eff, err := s.EffectivePermissions(ctx, row.Path, info, nullableStr(owner))
		if err != nil {
			continue
		}
		if s.cfg.Auth != nil && !eff.Has(PermissionRead) {
			continue
		}

		if row.Path == dir {
			files = append(files, fileEntryFromRow(row, owner, eff))
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(row.Path, dir), "/")
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if len(parts) > 1 {
			name := parts[0]
			if existing, ok := dirs[name]; ok {
				existing.entry.Size += row.Size
				continue
			}
			dpath := NormalizePath(path.Join(dir, name))
			dPerms, _ := s.EffectivePermissions(ctx, dpath, info, nil)
			dirs[name] = &agg{entry: FileEntry{
				Path: dpath, Name: name, IsDir: true, Size: row.Size,
				Date: row.UpdatedAt.UTC().Format(time.RFC3339), Permissions: uint64(dPerms),
			}}
			continue
		}
		files = append(files, fileEntryFromRow(row, owner, eff))
	}

	out := make([]FileEntry, 0, len(dirs)+len(files))
	for _, d := range dirs {
		out = append(out, d.entry)
	}
	out = append(out, files...)
	return out, nil
}

func fileEntryFromRow(row db.File, owner string, eff Permissions) FileEntry {
	return FileEntry{
		Path: row.Path, Name: path.Base(row.Path), IsDir: false,
		Size: row.Size, ContentType: row.ContentType, Description: row.Description,
		OwnerID: owner, Date: row.UpdatedAt.UTC().Format(time.RFC3339), Permissions: uint64(eff),
	}
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Server) GetFileAPI(w http.ResponseWriter, r *http.Request) {
	p := s.filePathFromRequest(r)
	info := GetUserInfo(r)
	file, err := s.store.Q.GetFile(r.Context(), p)
	if err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	owner := ""
	if file.UserID.Valid {
		owner = file.UserID.String
	}
	perms, err := s.EffectivePermissions(r.Context(), p, info, nullableStr(owner))
	if err != nil || (s.cfg.Auth != nil && !perms.Has(PermissionRead)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	if r.URL.Query().Get("download") == "1" || r.URL.Query().Get("dl") == "1" {
		s.streamFile(w, r, p, file.ContentType, true)
		return
	}
	s.writeJSON(w, fileEntryFromRow(file, owner, perms), http.StatusOK)
}

func (s *Server) GetPath(w http.ResponseWriter, r *http.Request) {
	p := NormalizePath(r.URL.Path)
	info := GetUserInfo(r)

	file, fileErr := s.store.Q.GetFile(r.Context(), p)
	canRead := false
	if fileErr == nil {
		owner := ""
		if file.UserID.Valid {
			owner = file.UserID.String
		}
		perms, _ := s.EffectivePermissions(r.Context(), p, info, nullableStr(owner))
		canRead = s.cfg.Auth == nil || perms.Has(PermissionRead)
		if !canRead {
			canRead, _ = s.CanAnonymousRead(r.Context(), p)
		}
		if isBot(r) {
			if canRead {
				s.writeOG(w, r, ogPublic(file.Path, file.Description, file.ContentType, file.Size))
			} else {
				s.writeOG(w, r, ogPrivate())
			}
			return
		}
		if canRead && r.URL.Query().Get("preview") == "1" {
			s.serveImagePreview(w, r, p)
			return
		}
		if canRead && (!wantsHTML(r) || r.URL.Query().Get("dl") != "") {
			s.streamFile(w, r, p, file.ContentType, r.URL.Query().Get("dl") != "")
			return
		}
	} else {
		perms, _ := s.EffectivePermissions(r.Context(), p, info, nil)
		canRead = s.cfg.Auth == nil || perms.Has(PermissionRead)
		if !canRead {
			canRead, _ = s.CanAnonymousRead(r.Context(), p)
		}
		if isBot(r) {
			if canRead {
				s.writeOG(w, r, ogPublic(p, "", "", 0))
			} else {
				s.writeOG(w, r, ogPrivate())
			}
			return
		}
	}

	if !canRead && info == nil && s.cfg.Auth != nil {
		http.Redirect(w, r, "/api/login?rd="+r.URL.RequestURI(), http.StatusFound)
		return
	}

	if wantsJSON(r) {
		s.syncPrefix(r.Context(), p)
		entries, err := s.listDir(r.Context(), p, info)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "forbidden" {
				status = http.StatusForbidden
			}
			s.writeError(w, r, err, status)
			return
		}
		s.writeJSON(w, map[string]any{"path": p, "files": entries}, http.StatusOK)
		return
	}

	s.serveSPA(w, r)
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}

func (s *Server) streamFile(w http.ResponseWriter, r *http.Request, p, contentType string, download bool) {
	rc, info, err := s.storage.GetObject(r.Context(), p, 0, -1)
	if err != nil {
		s.writeError(w, r, err, http.StatusNotFound)
		return
	}
	defer rc.Close()
	if contentType == "" {
		contentType = info.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	if download {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+path.Base(p)+"\"")
	}
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	_, _ = io.Copy(w, rc)
}

func (s *Server) UploadFileAPI(w http.ResponseWriter, r *http.Request) {
	dir := s.filePathFromRequest(r)
	info := GetUserInfo(r)
	perms, err := s.EffectivePermissions(r.Context(), dir, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(PermissionCreate)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}

	var meta struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Size        int64  `json:"size"`
	}
	var filePart *multipart.Part
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
		switch part.FormName() {
		case "json":
			if err := json.NewDecoder(part).Decode(&meta); err != nil {
				s.writeError(w, r, err, http.StatusBadRequest)
				return
			}
		case "file":
			filePart = part
		}
		if filePart != nil && meta.Name != "" {
			break
		}
	}
	if filePart == nil || meta.Name == "" {
		s.writeError(w, r, errors.New("missing file"), http.StatusBadRequest)
		return
	}
	defer filePart.Close()

	target := NormalizePath(path.Join(dir, meta.Name))
	ct := filePart.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	if err := s.storage.PutObject(r.Context(), target, meta.Size, filePart, ct); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	var ownerID interface{ String() string }
	_ = ownerID
	params := db.UpsertFileParams{
		Path: target, Size: meta.Size, ContentType: ct, Description: meta.Description,
		CreatedAt: now, UpdatedAt: now,
	}
	if info != nil && info.Subject != "" && info.Subject != "guest" {
		params.UserID = nullString(&info.Subject)
	}
	if _, err := s.store.Q.UpsertFile(r.Context(), params); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) PatchFileAPI(w http.ResponseWriter, r *http.Request) {
	p := s.filePathFromRequest(r)
	info := GetUserInfo(r)
	file, err := s.store.Q.GetFile(r.Context(), p)
	if err != nil {
		s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
		return
	}
	owner := ""
	if file.UserID.Valid {
		owner = file.UserID.String
	}
	perms, err := s.EffectivePermissions(r.Context(), p, info, nullableStr(owner))
	if err != nil || (s.cfg.Auth != nil && !perms.Has(PermissionUpdate)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}

	var meta struct {
		Name        string `json:"name"`
		Dir         string `json:"dir"`
		Description string `json:"description"`
		Size        int64  `json:"size"`
	}
	mr, err := r.MultipartReader()
	if err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	var replace io.ReadCloser
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
		if part.FormName() == "json" {
			_ = json.NewDecoder(part).Decode(&meta)
		}
		if part.FormName() == "file" {
			replace = part
		}
	}

	newPath := p
	if meta.Name != "" || meta.Dir != "" {
		dir := path.Dir(p)
		if meta.Dir != "" {
			dir = NormalizePath(meta.Dir)
		}
		name := path.Base(p)
		if meta.Name != "" {
			name = meta.Name
		}
		newPath = NormalizePath(path.Join(dir, name))
	}
	desc := file.Description
	if meta.Description != "" || r.FormValue("description") != "" {
		desc = meta.Description
	}
	size := file.Size
	ct := file.ContentType
	if replace != nil {
		defer replace.Close()
		if meta.Size > 0 {
			size = meta.Size
		}
		if err := s.storage.PutObject(r.Context(), newPath, size, replace, ct); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		if newPath != p {
			_ = s.storage.DeleteObject(r.Context(), p)
		}
	} else if newPath != p {
		if err := s.storage.CopyObject(r.Context(), p, newPath); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		_ = s.storage.DeleteObject(r.Context(), p)
	}

	now := time.Now().UTC()
	if _, err := s.store.Q.UpdateFileMeta(r.Context(), db.UpdateFileMetaParams{
		Path: p, NewPath: newPath, Size: size, ContentType: ct, Description: desc, UpdatedAt: now,
	}); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	if newPath != p {
		_ = s.store.Q.DeleteACLForPath(r.Context(), p)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) MoveFilesAPI(w http.ResponseWriter, r *http.Request) {
	dest := NormalizePath(r.Header.Get("Destination"))
	if dest == "" {
		s.writeError(w, r, errors.New("missing Destination"), http.StatusBadRequest)
		return
	}
	info := GetUserInfo(r)
	createPerms, err := s.EffectivePermissions(r.Context(), dest, info, nil)
	if err != nil || (s.cfg.Auth != nil && !createPerms.Has(PermissionCreate)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	var names []string
	if err := json.NewDecoder(r.Body).Decode(&names); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	base := s.filePathFromRequest(r)
	for _, name := range names {
		src := NormalizePath(path.Join(base, name))
		file, err := s.store.Q.GetFile(r.Context(), src)
		if err != nil {
			continue
		}
		owner := ""
		if file.UserID.Valid {
			owner = file.UserID.String
		}
		delPerms, _ := s.EffectivePermissions(r.Context(), src, info, nullableStr(owner))
		if s.cfg.Auth != nil && !delPerms.Has(PermissionDelete) {
			continue
		}
		target := NormalizePath(path.Join(dest, path.Base(src)))
		if err := s.storage.CopyObject(r.Context(), src, target); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		_ = s.storage.DeleteObject(r.Context(), src)
		now := time.Now().UTC()
		_, _ = s.store.Q.UpdateFileMeta(r.Context(), db.UpdateFileMetaParams{
			Path: src, NewPath: target, Size: file.Size, ContentType: file.ContentType,
			Description: file.Description, UpdatedAt: now,
		})
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteFilesAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	base := s.filePathFromRequest(r)
	var names []string
	_ = json.NewDecoder(r.Body).Decode(&names)
	targets := names
	if len(targets) == 0 {
		targets = []string{""}
	}
	for _, name := range targets {
		p := base
		if name != "" {
			p = NormalizePath(path.Join(base, name))
		}
		file, err := s.store.Q.GetFile(r.Context(), p)
		if err != nil {
			// delete prefix
			_ = s.store.Q.DeleteFilesUnder(r.Context(), db.DeleteFilesUnderParams{Path: p, PathLike: LikeUnder(p)})
			continue
		}
		owner := ""
		if file.UserID.Valid {
			owner = file.UserID.String
		}
		perms, _ := s.EffectivePermissions(r.Context(), p, info, nullableStr(owner))
		if s.cfg.Auth != nil && !perms.Has(PermissionDelete) {
			s.writeError(w, r, fmt.Errorf("forbidden: %s", p), http.StatusForbidden)
			return
		}
		_ = s.storage.DeleteObject(r.Context(), p)
		_ = s.store.Q.DeleteFile(r.Context(), p)
		_ = s.store.Q.DeleteACLForPath(r.Context(), p)
		_ = s.store.Q.DeleteSharesForPath(r.Context(), p)
	}
	w.WriteHeader(http.StatusNoContent)
}
