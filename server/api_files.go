package server

import (
	"archive/zip"
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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/storage"
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
	return acl.NormalizePath(p)
}

// pathExists reports whether path is root, a stored file, or a virtual directory
// (has at least one file under it).
func (s *Server) pathExists(ctx context.Context, p string) (bool, error) {
	p = acl.NormalizePath(p)
	if p == "/" {
		return true, nil
	}
	if _, err := s.store.Q.GetFile(ctx, p); err == nil {
		return true, nil
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	rows, err := s.store.Q.ListFilesUnder(ctx, database.ListFilesUnderParams{
		Path:     p,
		PathLike: acl.LikeUnder(p),
	})
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

func (s *Server) listDir(ctx context.Context, dir string, info *UserInfo) ([]FileEntry, error) {
	dir = acl.NormalizePath(dir)
	if s.cfg.AuthEnabled() {
		perms, err := s.EffectivePermissions(ctx, dir, info, nil)
		if err != nil {
			return nil, err
		}
		if !perms.Has(acl.PermissionRead) {
			anon, _ := s.CanAnonymousRead(ctx, dir)
			// Guests may browse; children are still filtered by ACL (often empty).
			if !anon && !s.isGuest(info) {
				return nil, errors.New("forbidden")
			}
		}
	}

	rows, err := s.store.Q.ListFilesUnder(ctx, database.ListFilesUnderParams{
		Path:     dir,
		PathLike: acl.LikeUnder(dir),
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
		if s.cfg.AuthEnabled() && !eff.Has(acl.PermissionRead) {
			continue
		}

		if row.Path == dir {
			if storage.IsDirectory(row.ContentType) {
				continue
			}
			files = append(files, s.fileEntryFromRow(ctx, row, owner, eff))
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(row.Path, dir), "/")
		parts := strings.SplitN(rel, "/", 2)
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if len(parts) > 1 || storage.IsDirectory(row.ContentType) {
			name := parts[0]
			updated := row.UpdatedAt.UTC().Format(time.RFC3339)
			size := row.Size
			if storage.IsDirectory(row.ContentType) {
				size = 0
			}
			if existing, ok := dirs[name]; ok {
				existing.entry.Size += size
				if updated > existing.entry.Date {
					existing.entry.Date = updated
				}
				continue
			}
			dpath := acl.NormalizePath(path.Join(dir, name))
			dPerms, _ := s.EffectivePermissions(ctx, dpath, info, nil)
			dirs[name] = &agg{entry: FileEntry{
				Path: dpath, Name: name, IsDir: true, Size: size,
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

func (s *Server) fileEntryFromRow(ctx context.Context, row database.File, owner string, eff acl.Permissions) FileEntry {
	entry := FileEntry{
		Path: row.Path, Name: path.Base(row.Path), IsDir: storage.IsDirectory(row.ContentType),
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
	p := acl.NormalizePath(r.URL.Path)
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
		canRead = !s.cfg.AuthEnabled() || perms.Has(acl.PermissionRead)
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
		if canRead && r.URL.Query().Get("preview") == "1" && !storage.IsDirectory(file.ContentType) {
			s.serveImagePreview(w, r, p)
			return
		}
		// Real files always stream (inline or attachment); SPA is only for directories.
		if canRead && !storage.IsDirectory(file.ContentType) {
			s.streamFile(w, r, p, file.ContentType, wantsDownload(r))
			return
		}
	} else {
		perms, _ := s.EffectivePermissions(r.Context(), p, info, nil)
		canRead = !s.cfg.AuthEnabled() || perms.Has(acl.PermissionRead)
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

	if !canRead && info == nil && s.cfg.AuthEnabled() {
		// JSON/XHR clients cannot follow the OIDC redirect; return 401 so the SPA can send the browser to login.
		if wantsJSON(r) || !wantsHTML(r) {
			s.writeError(w, r, errors.New("unauthorized"), http.StatusUnauthorized)
			return
		}
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

	isFolder := p == "/" || fileErr != nil || storage.IsDirectory(file.ContentType)
	if canRead && wantsDownload(r) && isFolder {
		s.streamZip(w, r, p, info, false)
		return
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
		dirPerms, _ := s.EffectivePermissions(r.Context(), p, info, nil)
		s.writeJSON(w, map[string]any{
			"path":        p,
			"files":       entries,
			"permissions": uint64(dirPerms),
		}, http.StatusOK)
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

func zipEntryName(root, filePath string, isDir bool) string {
	root = acl.NormalizePath(root)
	filePath = acl.NormalizePath(filePath)
	base := path.Base(root)
	if root == "/" {
		base = "files"
	}
	rel := strings.TrimPrefix(filePath, root)
	rel = strings.TrimPrefix(rel, "/")
	name := base
	if rel != "" {
		name = base + "/" + rel
	}
	if isDir && !strings.HasSuffix(name, "/") {
		name += "/"
	}
	return name
}

func (s *Server) streamZip(w http.ResponseWriter, r *http.Request, root string, info *UserInfo, skipACL bool) {
	root = acl.NormalizePath(root)
	rows, err := s.store.Q.ListFilesUnder(r.Context(), database.ListFilesUnderParams{
		Path:     root,
		PathLike: acl.LikeUnder(root),
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}

	filename := path.Base(root) + ".zip"
	if root == "/" {
		filename = "files.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(filename, `"`, ``)+`"`)
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	defer zw.Close()

	added := map[string]struct{}{}
	for _, row := range rows {
		owner := ""
		if row.UserID.Valid {
			owner = row.UserID.String
		}
		if !skipACL && s.cfg.AuthEnabled() {
			perms, err := s.EffectivePermissions(r.Context(), row.Path, info, nullableStr(owner))
			if err != nil || !perms.Has(acl.PermissionRead) {
				continue
			}
		}
		isDir := storage.IsDirectory(row.ContentType)
		if row.Path == root && isDir {
			continue
		}
		name := zipEntryName(root, row.Path, isDir)
		if _, ok := added[name]; ok {
			continue
		}
		added[name] = struct{}{}

		hdr := &zip.FileHeader{
			Name:     name,
			Method:   zip.Deflate,
			Modified: row.UpdatedAt.UTC(),
		}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return
		}
		if isDir {
			continue
		}
		rc, _, err := s.storage.GetObject(r.Context(), row.Path, 0, -1)
		if err != nil {
			continue
		}
		_, _ = io.Copy(fw, rc)
		_ = rc.Close()
	}
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
	p := s.filePathFromRequest(r)
	info := GetUserInfo(r)
	ct := r.Header.Get("Content-Type")

	// Multipart into a directory (browser form: json name + file part).
	if strings.Contains(ct, "multipart/") {
		s.uploadMultipartIntoDir(w, r, p, info)
		return
	}

	// Raw body → small file at this path. Empty body → mkdir at this path.
	if r.ContentLength > 0 {
		s.putRawFile(w, r, p, info, r.ContentLength, r.Body, ct, "")
		return
	}
	s.mkdirPath(w, r, p, info)
}

func (s *Server) putRawFile(w http.ResponseWriter, r *http.Request, target string, info *UserInfo, size int64, body io.Reader, contentType, description string) {
	target = acl.NormalizePath(target)
	if target == "/" || acl.IsReservedPath(target) {
		s.writeError(w, r, errors.New("invalid path"), http.StatusBadRequest)
		return
	}
	if size < 0 {
		s.writeError(w, r, errors.New("content-length required"), http.StatusBadRequest)
		return
	}
	if size > s.cfg.Upload.MaxSize.Bytes {
		s.writeError(w, r, errors.New("file too large"), http.StatusBadRequest)
		return
	}
	if size > s.cfg.Upload.ChunkSize.Bytes {
		s.writeError(w, r, errors.New("use resumable upload for large files"), http.StatusBadRequest)
		return
	}

	parent := path.Dir(target)
	existing, getErr := s.store.Q.GetFile(r.Context(), target)
	switch {
	case getErr == nil:
		owner := ""
		if existing.UserID.Valid {
			owner = existing.UserID.String
		}
		up, uerr := s.EffectivePermissions(r.Context(), target, info, nullableStr(owner))
		if uerr != nil || (s.cfg.AuthEnabled() && !up.Has(acl.PermissionUpdate)) {
			s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
			return
		}
	case errors.Is(getErr, sql.ErrNoRows):
		parentPerms, err := s.EffectivePermissions(r.Context(), parent, info, nil)
		if err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		if s.cfg.AuthEnabled() && !parentPerms.Has(acl.PermissionCreate) {
			s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
			return
		}
	default:
		s.writeError(w, r, getErr, http.StatusInternalServerError)
		return
	}

	if err := s.ensureDir(r.Context(), parent, info); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errForbidden) {
			status = http.StatusForbidden
		}
		s.writeError(w, r, err, status)
		return
	}

	ct := sniffContentType(path.Base(target), contentType)
	if err := s.storage.PutObject(r.Context(), target, size, body, ct); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	params := database.UpsertFileParams{
		Path: target, Size: size, ContentType: ct, Description: description,
		CreatedAt: now, UpdatedAt: now,
	}
	if getErr == nil {
		params.CreatedAt = existing.CreatedAt
		params.UserID = existing.UserID
		if description == "" {
			params.Description = existing.Description
		}
	} else if info != nil && info.Subject != "" && info.Subject != "guest" {
		params.UserID = database.NullString(&info.Subject)
	}
	if _, err := s.store.Q.UpsertFile(r.Context(), params); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) uploadMultipartIntoDir(w http.ResponseWriter, r *http.Request, dir string, info *UserInfo) {
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

	target, err := resolveUploadTarget(dir, meta.Name)
	if err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	s.putRawFile(w, r, target, info, meta.Size, filePart, filePart.Header.Get("Content-Type"), meta.Description)
}

func (s *Server) mkdirPath(w http.ResponseWriter, r *http.Request, target string, info *UserInfo) {
	target = acl.NormalizePath(target)
	if target == "/" {
		s.writeError(w, r, errors.New("invalid folder name"), http.StatusBadRequest)
		return
	}
	if acl.IsReservedPath(target) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	perms, err := s.EffectivePermissions(r.Context(), path.Dir(target), info, nil)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	if s.cfg.AuthEnabled() && !perms.Has(acl.PermissionCreate) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	if _, err := s.store.Q.GetFile(r.Context(), target); err == nil {
		s.writeError(w, r, errors.New("already exists"), http.StatusConflict)
		return
	}
	now := time.Now().UTC()
	for _, anc := range acl.AncestorPathsRootFirst(target) {
		if anc == "/" || anc == target {
			continue
		}
		if _, err := s.store.Q.GetFile(r.Context(), anc); err == nil {
			continue
		}
		if err := s.storage.Mkdir(r.Context(), anc); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		if _, err := s.store.Q.UpsertFile(r.Context(), dirFileParams(anc, now, "")); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
	}
	if err := s.storage.Mkdir(r.Context(), target); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	owner := ""
	if info != nil {
		owner = info.Subject
	}
	if _, err := s.store.Q.UpsertFile(r.Context(), dirFileParams(target, now, owner)); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, map[string]any{"path": target, "is_dir": true}, http.StatusCreated)
}

func validEntryName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, "/\\")
}

// resolveRenameTarget maps a rename spec to a destination path.
// A leading slash is absolute; otherwise it is relative to the source's parent
// (so ../x moves up and a/b moves deeper).
func resolveRenameTarget(from, spec string) (string, error) {
	spec = strings.TrimSpace(strings.ReplaceAll(spec, "\\", "/"))
	if spec == "" {
		return "", errors.New("invalid name")
	}
	var to string
	if strings.HasPrefix(spec, "/") {
		to = acl.NormalizePath(spec)
	} else {
		to = acl.NormalizePath(path.Join(path.Dir(from), spec))
	}
	if to == "/" || !validEntryName(path.Base(to)) {
		return "", errors.New("invalid name")
	}
	if acl.IsReservedPath(to) {
		return "", errors.New("reserved path")
	}
	return to, nil
}

var errForbidden = errors.New("forbidden")

func (s *Server) ensureDir(ctx context.Context, destDir string, info *UserInfo) error {
	return s.ensureDirWithOpts(ctx, destDir, info, true)
}

// ensureDirUnchecked creates missing parent directories without ACL checks (share upload finalize).
func (s *Server) ensureDirUnchecked(ctx context.Context, destDir, ownerSubject string) error {
	var info *UserInfo
	if ownerSubject != "" {
		info = &UserInfo{Subject: ownerSubject}
	}
	return s.ensureDirWithOpts(ctx, destDir, info, false)
}

func dirFileParams(p string, now time.Time, ownerSubject string) database.UpsertFileParams {
	params := database.UpsertFileParams{
		Path: p, Size: 0, ContentType: storage.ContentTypeDirectory,
		CreatedAt: now, UpdatedAt: now,
	}
	if ownerSubject != "" && ownerSubject != "guest" {
		params.UserID = database.NullString(&ownerSubject)
	}
	return params
}

func (s *Server) ensureDirWithOpts(ctx context.Context, destDir string, info *UserInfo, checkACL bool) error {
	destDir = acl.NormalizePath(destDir)
	if destDir == "/" {
		return nil
	}
	if acl.IsReservedPath(destDir) {
		return errors.New("reserved path")
	}
	var missing []string
	for p := destDir; p != "/"; p = path.Dir(p) {
		file, err := s.store.Q.GetFile(ctx, p)
		if err == nil {
			if !storage.IsDirectory(file.ContentType) {
				return errors.New("destination is not a folder")
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
		if ok {
			break
		}
		missing = append(missing, p)
	}
	now := time.Now().UTC()
	owner := ""
	if info != nil {
		owner = info.Subject
	}
	for i := len(missing) - 1; i >= 0; i-- {
		p := missing[i]
		if checkACL && s.cfg.AuthEnabled() {
			perms, err := s.EffectivePermissions(ctx, path.Dir(p), info, nil)
			if err != nil || !perms.Has(acl.PermissionCreate) {
				return errForbidden
			}
		}
		if err := s.storage.Mkdir(ctx, p); err != nil {
			return err
		}
		if _, err := s.store.Q.UpsertFile(ctx, dirFileParams(p, now, owner)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) authorizeRename(ctx context.Context, from, to string, info *UserInfo, srcOwner *string) error {
	if !s.cfg.AuthEnabled() {
		return nil
	}
	srcPerms, err := s.EffectivePermissions(ctx, from, info, srcOwner)
	if err != nil {
		return err
	}
	if path.Dir(from) == path.Dir(to) {
		if !srcPerms.Has(acl.PermissionUpdate) {
			return errForbidden
		}
		return nil
	}
	if !srcPerms.Has(acl.PermissionDelete) {
		return errForbidden
	}
	destPerms, err := s.EffectivePermissions(ctx, path.Dir(to), info, nil)
	if err != nil {
		return err
	}
	if !destPerms.Has(acl.PermissionCreate) {
		return errForbidden
	}
	return nil
}

func (s *Server) remapPrefix(ctx context.Context, from, to string) error {
	now := time.Now().UTC()
	files, err := s.store.Q.ListFilesUnder(ctx, database.ListFilesUnderParams{Path: from, PathLike: acl.LikeUnder(from)})
	if err != nil {
		return err
	}
	sort.Slice(files, func(i, j int) bool { return len(files[i].Path) > len(files[j].Path) })
	for _, f := range files {
		newPath := acl.RemapPrefix(f.Path, from, to)
		if newPath == f.Path {
			continue
		}
		if _, err := s.store.Q.UpdateFileMeta(ctx, database.UpdateFileMetaParams{
			Path: f.Path, NewPath: newPath, Size: f.Size, ContentType: f.ContentType,
			Description: f.Description, UpdatedAt: now,
		}); err != nil {
			return err
		}
	}
	acls, err := s.store.Q.ListACLUnder(ctx, database.ListACLUnderParams{Path: from, PathLike: acl.LikeUnder(from)})
	if err != nil {
		return err
	}
	for _, row := range acls {
		newPath := acl.RemapPrefix(row.Path, from, to)
		if newPath == row.Path {
			continue
		}
		if _, err := s.store.Q.UpsertACL(ctx, database.UpsertACLParams{
			Path: newPath, PrincipalType: row.PrincipalType, PrincipalID: row.PrincipalID,
			Allow: row.Allow, Deny: row.Deny,
		}); err != nil {
			return err
		}
		_ = s.store.Q.DeleteACL(ctx, database.DeleteACLParams{
			Path: row.Path, PrincipalType: row.PrincipalType, PrincipalID: row.PrincipalID,
		})
	}
	shares, err := s.store.Q.ListSharesByPath(ctx, database.ListSharesByPathParams{Path: from, PathLike: acl.LikeUnder(from)})
	if err != nil {
		return err
	}
	for _, sh := range shares {
		newPath := acl.RemapPrefix(sh.Path, from, to)
		if newPath == sh.Path {
			continue
		}
		if err := s.store.Q.UpdateSharePath(ctx, database.UpdateSharePathParams{ID: sh.ID, Path: newPath}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) renamePath(ctx context.Context, from, to string, info *UserInfo) error {
	from, to = acl.NormalizePath(from), acl.NormalizePath(to)
	if from == "/" || to == "/" {
		return errors.New("cannot rename root")
	}
	if from == to {
		return nil
	}
	if acl.IsSelfOrUnder(to, from) {
		return errors.New("cannot move into itself")
	}
	if acl.IsReservedPath(to) {
		return errors.New("reserved path")
	}
	if err := s.ensureDir(ctx, path.Dir(to), info); err != nil {
		return err
	}
	existing, err := s.store.Q.ListFilesUnder(ctx, database.ListFilesUnderParams{Path: to, PathLike: acl.LikeUnder(to)})
	if err != nil {
		return err
	}
	for _, row := range existing {
		if !acl.IsSelfOrUnder(row.Path, from) {
			return errAlreadyExists
		}
	}
	if err := s.storage.RenameObject(ctx, from, to); err != nil {
		return err
	}
	return s.remapPrefix(ctx, from, to)
}

func renameHTTPStatus(err error) int {
	switch {
	case errors.Is(err, errAlreadyExists):
		return http.StatusConflict
	case errors.Is(err, errForbidden):
		return http.StatusForbidden
	}
	msg := err.Error()
	if strings.Contains(msg, "cannot") || strings.Contains(msg, "invalid") || strings.Contains(msg, "reserved") || strings.Contains(msg, "not a folder") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

var errAlreadyExists = errors.New("already exists")

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

	var meta struct {
		Name        string `json:"name"`
		Dir         string `json:"dir"`
		Description string `json:"description"`
		Size        int64  `json:"size"`
	}
	var replace io.ReadCloser
	ctHeader := r.Header.Get("Content-Type")
	if strings.Contains(ctHeader, "application/json") {
		if err := decodeJSONOptional(r, &meta); err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
	} else {
		mr, err := r.MultipartReader()
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
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
	}

	newPath := p
	if strings.TrimSpace(meta.Dir) != "" {
		dir := acl.NormalizePath(meta.Dir)
		name := path.Base(p)
		if strings.TrimSpace(meta.Name) != "" {
			name = strings.TrimSpace(meta.Name)
		}
		if !validEntryName(name) {
			s.writeError(w, r, errors.New("invalid name"), http.StatusBadRequest)
			return
		}
		newPath = acl.NormalizePath(path.Join(dir, name))
	} else if strings.TrimSpace(meta.Name) != "" {
		newPath, err = resolveRenameTarget(p, meta.Name)
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
	}
	if acl.IsReservedPath(newPath) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}

	descChanged := meta.Description != "" || r.FormValue("description") != ""
	if replace != nil || descChanged || newPath == p {
		perms, err := s.EffectivePermissions(r.Context(), p, info, nullableStr(owner))
		if err != nil || (s.cfg.AuthEnabled() && !perms.Has(acl.PermissionUpdate)) {
			s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
			return
		}
	}
	if newPath != p {
		if err := s.authorizeRename(r.Context(), p, newPath, info, nullableStr(owner)); err != nil {
			s.writeError(w, r, err, renameHTTPStatus(err))
			return
		}
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
		if newPath != p {
			if err := s.renamePath(r.Context(), p, newPath, info); err != nil {
				s.writeError(w, r, err, renameHTTPStatus(err))
				return
			}
			p = newPath
		}
		if err := s.storage.PutObject(r.Context(), newPath, size, replace, ct); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		now := time.Now().UTC()
		if _, err := s.store.Q.UpdateFileMeta(r.Context(), database.UpdateFileMetaParams{
			Path: newPath, NewPath: newPath, Size: size, ContentType: ct, Description: desc, UpdatedAt: now,
		}); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if newPath != p {
		if err := s.renamePath(r.Context(), p, newPath, info); err != nil {
			s.writeError(w, r, err, renameHTTPStatus(err))
			return
		}
		p = newPath
	}
	if desc != file.Description {
		now := time.Now().UTC()
		if _, err := s.store.Q.UpdateFileMeta(r.Context(), database.UpdateFileMetaParams{
			Path: p, NewPath: p, Size: size, ContentType: ct, Description: desc, UpdatedAt: now,
		}); err != nil {
			s.writeError(w, r, err, http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) MoveFilesAPI(w http.ResponseWriter, r *http.Request) {
	dest := acl.NormalizePath(r.Header.Get("Destination"))
	if dest == "" {
		s.writeError(w, r, errors.New("missing Destination"), http.StatusBadRequest)
		return
	}
	if acl.IsReservedPath(dest) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	info := GetUserInfo(r)
	createPerms, err := s.EffectivePermissions(r.Context(), dest, info, nil)
	if err != nil || (s.cfg.AuthEnabled() && !createPerms.Has(acl.PermissionCreate)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	var names []string
	if err := decodeJSONOptional(r, &names); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	base := s.filePathFromRequest(r)
	// No body → move the request path itself into Destination.
	var srcs []string
	if len(names) == 0 {
		if base == "/" {
			s.writeError(w, r, errors.New("cannot move root"), http.StatusBadRequest)
			return
		}
		srcs = []string{base}
	} else {
		for _, name := range names {
			srcs = append(srcs, acl.NormalizePath(path.Join(base, name)))
		}
	}
	for _, src := range srcs {
		file, err := s.store.Q.GetFile(r.Context(), src)
		if err != nil {
			continue
		}
		owner := ""
		if file.UserID.Valid {
			owner = file.UserID.String
		}
		delPerms, _ := s.EffectivePermissions(r.Context(), src, info, nullableStr(owner))
		if s.cfg.AuthEnabled() && !delPerms.Has(acl.PermissionDelete) {
			continue
		}
		target := acl.NormalizePath(path.Join(dest, path.Base(src)))
		if err := s.renamePath(r.Context(), src, target, info); err != nil {
			s.writeError(w, r, err, renameHTTPStatus(err))
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteFilesAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	base := s.filePathFromRequest(r)
	var names []string
	_ = decodeJSONOptional(r, &names)
	targets := names
	if len(targets) == 0 {
		targets = []string{""}
	}
	for _, name := range targets {
		p := base
		if name != "" {
			p = acl.NormalizePath(path.Join(base, name))
		}
		file, err := s.store.Q.GetFile(r.Context(), p)
		owner := ""
		if err == nil && file.UserID.Valid {
			owner = file.UserID.String
		}
		perms, _ := s.EffectivePermissions(r.Context(), p, info, nullableStr(owner))
		if s.cfg.AuthEnabled() && !perms.Has(acl.PermissionDelete) {
			s.writeError(w, r, fmt.Errorf("forbidden: %s", p), http.StatusForbidden)
			return
		}
		rows, _ := s.store.Q.ListFilesUnder(r.Context(), database.ListFilesUnderParams{Path: p, PathLike: acl.LikeUnder(p)})
		for _, row := range rows {
			_ = s.storage.DeleteObject(r.Context(), row.Path)
		}
		_ = s.storage.DeleteObject(r.Context(), p)
		_ = s.store.Q.DeleteFilesUnder(r.Context(), database.DeleteFilesUnderParams{Path: p, PathLike: acl.LikeUnder(p)})
		_ = s.store.Q.DeleteACLForPath(r.Context(), p)
		_ = s.store.Q.DeleteSharesForPath(r.Context(), p)
	}
	w.WriteHeader(http.StatusNoContent)
}
