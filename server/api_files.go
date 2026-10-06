package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/database/dbsqlc"
)

type FileEntry struct {
	Path        string `json:"path"`
	Name        string `json:"name"`
	IsDir       bool   `json:"is_dir"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	Description string `json:"description,omitempty"`
	OwnerID     string `json:"owner_id,omitempty"`
	Owner       string `json:"owner,omitempty"`
	Date        string `json:"date"`
	Permissions uint64 `json:"permissions"`
}

func (s *Server) filePathFromRequest(r *http.Request) string {
	p := chi.URLParam(r, "*")
	if p == "" {
		p = r.URL.Path
	}
	return NormalizePath(p)
}

// pathExists reports whether path is root, a stored file, or a virtual directory
// (has at least one file under it).
func (s *Server) pathExists(ctx context.Context, p string) (bool, error) {
	p = NormalizePath(p)
	if p == "/" {
		return true, nil
	}
	if _, err := s.store.Q.GetFile(ctx, p); err == nil {
		return true, nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	rows, err := s.store.Q.ListFilesUnder(ctx, dbsqlc.ListFilesUnderParams{
		Path:     p,
		PathLike: LikeUnder(p),
	})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
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
			// Guests may browse; children are still filtered by ACL (often empty).
			if !anon && !s.isGuest(info) {
				return nil, errors.New("forbidden")
			}
		}
	}

	rows, err := s.store.Q.ListFilesUnder(ctx, dbsqlc.ListFilesUnderParams{
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
			files = append(files, s.fileEntryFromRow(ctx, row, owner, eff))
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(row.Path, dir), "/")
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if len(parts) > 1 {
			name := parts[0]
			updated := row.UpdatedAt.UTC().Format(time.RFC3339)
			if existing, ok := dirs[name]; ok {
				existing.entry.Size += row.Size
				if updated > existing.entry.Date {
					existing.entry.Date = updated
				}
				continue
			}
			dpath := NormalizePath(path.Join(dir, name))
			dPerms, _ := s.EffectivePermissions(ctx, dpath, info, nil)
			dirs[name] = &agg{entry: FileEntry{
				Path: dpath, Name: name, IsDir: true, Size: row.Size,
				Date: updated, Permissions: uint64(dPerms),
			}}
			continue
		}
		files = append(files, s.fileEntryFromRow(ctx, row, owner, eff))
	}

	out := make([]FileEntry, 0, len(dirs)+len(files))
	for _, d := range dirs {
		out = append(out, d.entry)
	}
	out = append(out, files...)
	return out, nil
}

func (s *Server) fileEntryFromRow(ctx context.Context, row dbsqlc.File, owner string, eff Permissions) FileEntry {
	entry := FileEntry{
		Path: row.Path, Name: path.Base(row.Path), IsDir: false,
		Size: row.Size, ContentType: row.ContentType, Description: row.Description,
		OwnerID: owner, Date: row.UpdatedAt.UTC().Format(time.RFC3339), Permissions: uint64(eff),
	}
	if owner != "" {
		if user, err := s.store.Q.GetUser(ctx, owner); err == nil {
			entry.Owner = user.Username
		}
	}
	return entry
}

func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Server) GetPath(w http.ResponseWriter, r *http.Request) {
	p := NormalizePath(r.URL.Path)
	info := GetUserInfo(r)

	// Pick up files that exist on disk but aren't indexed yet before deciding SPA vs stream.
	s.syncPrefix(r.Context(), p)

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
		// Real files always stream (inline or attachment); SPA is only for directories.
		if canRead {
			s.streamFile(w, r, p, file.ContentType, wantsDownload(r))
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

	if fileErr != nil {
		exists, err := s.pathExists(r.Context(), p)
		if err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		if !exists {
			if wantsJSON(r) {
				s.writeError(w, r, errors.New("not found"), http.StatusNotFound)
				return
			}
			if p != "/" {
				http.Redirect(w, r, "/", http.StatusFound)
				return
			}
		}
	}

	if wantsJSON(r) {
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

var contentTypeByExt = map[string]string{
	".mp3":  "audio/mpeg",
	".m4a":  "audio/mp4",
	".aac":  "audio/aac",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
	".flac": "audio/flac",
	".opus": "audio/opus",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".ogv":  "video/ogg",
	".mov":  "video/quicktime",
	".m4v":  "video/x-m4v",
	".pdf":  "application/pdf",
}

func sniffContentType(filePath, contentType string) string {
	if contentType != "" && contentType != "application/octet-stream" {
		return contentType
	}
	ext := strings.ToLower(path.Ext(filePath))
	if byExt, ok := contentTypeByExt[ext]; ok {
		return byExt
	}
	if ext != "" {
		if byExt := mime.TypeByExtension(ext); byExt != "" {
			return byExt
		}
	}
	if contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func wantsDownload(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("dl") == "1" || q.Get("download") == "1"
}

func (s *Server) streamFile(w http.ResponseWriter, r *http.Request, p, contentType string, download bool) {
	if wantsDownload(r) {
		download = true
	}
	info, err := s.storage.Stat(r.Context(), p)
	if err != nil {
		s.writeError(w, r, err, http.StatusNotFound)
		return
	}
	contentType = sniffContentType(p, contentType)
	if contentType == "application/octet-stream" {
		contentType = sniffContentType(p, info.ContentType)
	}

	size := info.Size
	start, end := int64(0), int64(-1)
	status := http.StatusOK
	// Range seeks are for inline playback; forced downloads send the full object.
	if !download {
		if rangeHeader := r.Header.Get("Range"); rangeHeader != "" && strings.HasPrefix(rangeHeader, "bytes=") && size > 0 {
			spec := strings.TrimPrefix(rangeHeader, "bytes=")
			parts := strings.SplitN(spec, "-", 2)
			if len(parts) == 2 {
				if parts[0] != "" {
					if n, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
						start = n
					}
				}
				if parts[1] != "" {
					if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
						end = n
					}
				}
				if end < 0 || end >= size {
					end = size - 1
				}
				if start < 0 || start >= size || start > end {
					w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
					http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
					return
				}
				status = http.StatusPartialContent
			}
		}
	}

	rc, _, err := s.storage.GetObject(r.Context(), p, start, end)
	if err != nil {
		s.writeError(w, r, err, http.StatusNotFound)
		return
	}
	defer rc.Close()

	name := path.Base(p)
	if download {
		// Force save-as even for playable MIME types (audio/video/pdf).
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
		w.Header().Set("X-Content-Type-Options", "nosniff")
	} else {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Disposition", "inline; filename=\""+name+"\"")
		w.Header().Set("Accept-Ranges", "bytes")
	}

	if status == http.StatusPartialContent {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(status)
	} else if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
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
	if IsReservedPath(target) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	ct := sniffContentType(meta.Name, filePart.Header.Get("Content-Type"))
	if err := s.storage.PutObject(r.Context(), target, meta.Size, filePart, ct); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	var ownerID interface{ String() string }
	_ = ownerID
	params := dbsqlc.UpsertFileParams{
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
	if IsReservedPath(newPath) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
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
	if _, err := s.store.Q.UpdateFileMeta(r.Context(), dbsqlc.UpdateFileMetaParams{
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
	if IsReservedPath(dest) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
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
		if IsReservedPath(target) {
			s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
			return
		}
		if err := s.storage.CopyObject(r.Context(), src, target); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		_ = s.storage.DeleteObject(r.Context(), src)
		now := time.Now().UTC()
		_, _ = s.store.Q.UpdateFileMeta(r.Context(), dbsqlc.UpdateFileMetaParams{
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
			_ = s.store.Q.DeleteFilesUnder(r.Context(), dbsqlc.DeleteFilesUnderParams{Path: p, PathLike: LikeUnder(p)})
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
