package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/database/dbq"
	"github.com/topi314/godrive/server/storage"
)

func shareToJSON(share dbq.Share) map[string]any {
	out := map[string]any{
		"id":         share.ID,
		"path":       share.Path,
		"url":        "/s/" + share.ID,
		"created_at": share.CreatedAt.UTC(),
		"expires_at": nil,
	}
	if share.ExpiresAt.Valid {
		t := share.ExpiresAt.Time.UTC()
		out["expires_at"] = t
	}
	return out
}

func parseExpiresIn(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, nil
	}
	if d, err := time.ParseDuration(raw); err == nil {
		if d <= 0 {
			return 0, errors.New("invalid expiry")
		}
		return d, nil
	}
	if strings.HasSuffix(raw, "d") {
		n, err := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(raw, "d")))
		if err != nil || n <= 0 {
			return 0, errors.New("invalid expiry")
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return 0, errors.New("invalid expiry")
}

func (s *Server) CreateShareAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	var body struct {
		Path      string     `json:"path"`
		ExpiresAt *time.Time `json:"expires_at"`
		ExpiresIn string     `json:"expires_in"`
		Allow     *int64     `json:"allow"`
		Deny      int64      `json:"deny"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	p := acl.NormalizePath(body.Path)
	if acl.IsReservedPath(p) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(acl.PermissionShare)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	allow := int64(acl.PermissionRead)
	if body.Allow != nil {
		allow = *body.Allow
	}
	if allow == 0 {
		s.writeError(w, r, errors.New("allow must be non-zero"), http.StatusBadRequest)
		return
	}
	now := time.Now().UTC()
	var expiresAt *time.Time
	if strings.TrimSpace(body.ExpiresIn) != "" {
		d, err := parseExpiresIn(body.ExpiresIn)
		if err != nil {
			s.writeError(w, r, err, http.StatusBadRequest)
			return
		}
		t := now.Add(d)
		expiresAt = &t
	} else if body.ExpiresAt != nil {
		t := body.ExpiresAt.UTC()
		if !t.After(now) {
			s.writeError(w, r, errors.New("expiry must be in the future"), http.StatusBadRequest)
			return
		}
		expiresAt = &t
	}
	id := s.newShareID()
	share, err := s.store.Q.CreateShare(r.Context(), dbq.CreateShareParams{
		ID: id, Path: p, UserID: info.Subject, CreatedAt: now, ExpiresAt: database.NullTime(expiresAt),
	})
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	if _, err := s.store.Q.UpsertACL(r.Context(), dbq.UpsertACLParams{
		Path: p, PrincipalType: acl.PrincipalShare, PrincipalID: id,
		Allow: allow, Deny: body.Deny,
	}); err != nil {
		_ = s.store.Q.DeleteShare(r.Context(), id)
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	out := shareToJSON(share)
	out["allow"] = allow
	out["deny"] = body.Deny
	s.writeJSON(w, out, http.StatusCreated)
}

func (s *Server) ListSharesAPI(w http.ResponseWriter, r *http.Request) {
	info := GetUserInfo(r)
	shares, err := s.store.Q.ListSharesByUser(r.Context(), info.Subject)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	out := make([]map[string]any, 0, len(shares))
	for _, share := range shares {
		out = append(out, shareToJSON(share))
	}
	s.writeJSON(w, out, http.StatusOK)
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
	_ = s.store.Q.DeleteACLForPrincipal(r.Context(), dbq.DeleteACLForPrincipalParams{
		PrincipalType: acl.PrincipalShare, PrincipalID: id,
	})
	_ = s.store.Q.DeleteShare(r.Context(), id)
	w.WriteHeader(http.StatusNoContent)
}

// underShare reports whether target is exactly root or a descendant of root.
func underShare(root, target string) bool {
	root = acl.NormalizePath(root)
	target = acl.NormalizePath(target)
	if root == "/" {
		return true
	}
	return target == root || strings.HasPrefix(target, root+"/")
}

func invalidShareSegment(seg string) bool {
	if seg == "" || seg == "." || seg == ".." {
		return false // handled by caller
	}
	if strings.ContainsAny(seg, "\\\x00:") {
		return true
	}
	// Block weird dot-only / encoded-looking traversal probes.
	if seg == "..." || strings.EqualFold(seg, "%2e%2e") || strings.EqualFold(seg, "%2e") {
		return true
	}
	return false
}

// joinShareTarget resolves a share-relative rest path under shareRoot.
// Rejects ".." escapes above the share root and other traversal probes.
func joinShareTarget(shareRoot, rest string) (string, error) {
	root := acl.NormalizePath(shareRoot)
	if strings.Contains(rest, "\x00") {
		return "", errors.New("forbidden")
	}
	// Normalize separators; never treat rest as an absolute path.
	rest = strings.ReplaceAll(rest, "\\", "/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return root, nil
	}
	var parts []string
	for _, seg := range strings.Split(rest, "/") {
		if seg == "" || seg == "." {
			continue
		}
		if seg == ".." {
			if len(parts) == 0 {
				return "", errors.New("forbidden")
			}
			parts = parts[:len(parts)-1]
			continue
		}
		if invalidShareSegment(seg) {
			return "", errors.New("forbidden")
		}
		parts = append(parts, seg)
	}
	target := root
	if len(parts) > 0 {
		target = acl.NormalizePath(path.Join(append([]string{root}, parts...)...))
	}
	if !underShare(root, target) {
		return "", errors.New("forbidden")
	}
	return target, nil
}

var errShareExpired = errors.New("expired")

func (s *Server) resolveShare(r *http.Request) (dbq.Share, string, error) {
	id := chi.URLParam(r, "id")
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return dbq.Share{}, "", errors.New("not found")
	}
	share, err := s.store.Q.GetShare(r.Context(), id)
	if err != nil {
		return share, "", err
	}
	if share.ExpiresAt.Valid && !share.ExpiresAt.Time.After(time.Now()) {
		return share, "", errShareExpired
	}
	// Prefer the wildcard path param; fall back to path after /s/{id}/.
	rest := chi.URLParam(r, "*")
	if rest == "" {
		prefix := "/s/" + id
		if p := r.URL.Path; strings.HasPrefix(p, prefix+"/") {
			rest = strings.TrimPrefix(p, prefix+"/")
		}
	}
	target, err := joinShareTarget(share.Path, rest)
	if err != nil {
		return share, "", err
	}
	if !underShare(share.Path, target) {
		return share, "", errors.New("forbidden")
	}
	return share, target, nil
}

// shareBrowsePath maps a storage path to the public /s/{id}/… browse URL.
func shareBrowsePath(share dbq.Share, storagePath string) string {
	root := acl.NormalizePath(share.Path)
	storagePath = acl.NormalizePath(storagePath)
	base := "/s/" + share.ID
	if !underShare(root, storagePath) {
		return base
	}
	if storagePath == root {
		return base
	}
	rel := strings.TrimPrefix(storagePath, root)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" {
		return base
	}
	return base + "/" + rel
}

func (s *Server) shareEffectivePerms(ctx context.Context, shareID, filePath string) (acl.Permissions, error) {
	paths := acl.AncestorPathsRootFirst(filePath)
	rows, err := s.store.ListACLByPaths(ctx, paths)
	if err != nil {
		return 0, err
	}
	return acl.CalculatePermissions(paths, rows, &acl.Identity{ShareID: shareID}), nil
}

// listShareDir lists immediate children under target for a share capability URL.
func (s *Server) listShareDir(ctx context.Context, share dbq.Share, target string, sharePerms acl.Permissions) ([]FileEntry, error) {
	target = acl.NormalizePath(target)
	if !underShare(share.Path, target) {
		return nil, errors.New("forbidden")
	}
	if !sharePerms.Has(acl.PermissionRead) {
		return nil, errors.New("forbidden")
	}
	s.syncPrefix(ctx, target)

	rows, err := s.store.Q.ListFilesUnder(ctx, dbq.ListFilesUnderParams{
		Path:     target,
		PathLike: acl.LikeUnder(target),
	})
	if err != nil {
		return nil, err
	}

	type agg struct{ entry FileEntry }
	dirs := map[string]*agg{}
	var files []FileEntry

	for _, row := range rows {
		if !underShare(share.Path, row.Path) {
			continue
		}
		if row.Path != target && !underShare(target, row.Path) {
			continue
		}
		owner := ""
		if row.UserID.Valid {
			owner = row.UserID.String
		}
		childPerms, _ := s.shareEffectivePerms(ctx, share.ID, row.Path)
		if !childPerms.Has(acl.PermissionRead) && row.Path != target {
			// Still show dirs that lead to readable content? Keep simple: require read on child path.
			dpath := row.Path
			if !storage.IsDirectory(row.ContentType) {
				rel := strings.TrimPrefix(strings.TrimPrefix(row.Path, target), "/")
				parts := strings.SplitN(rel, "/", 2)
				if len(parts) > 1 {
					dpath = acl.NormalizePath(path.Join(target, parts[0]))
					childPerms, _ = s.shareEffectivePerms(ctx, share.ID, dpath)
				}
			}
			if !childPerms.Has(acl.PermissionRead) {
				continue
			}
		}

		if row.Path == target {
			if storage.IsDirectory(row.ContentType) {
				continue
			}
			entry := s.fileEntryFromRow(ctx, row, owner, sharePerms)
			entry.Path = shareBrowsePath(share, row.Path)
			files = append(files, entry)
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(row.Path, target), "/")
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
			dpath := acl.NormalizePath(path.Join(target, name))
			dPerms, _ := s.shareEffectivePerms(ctx, share.ID, dpath)
			dirs[name] = &agg{entry: FileEntry{
				Path: shareBrowsePath(share, dpath), Name: name, IsDir: true, Size: size,
				Date: updated, Permissions: uint64(dPerms),
			}}
			continue
		}
		entry := s.fileEntryFromRow(ctx, row, owner, childPerms)
		entry.Path = shareBrowsePath(share, row.Path)
		files = append(files, entry)
	}

	out := make([]FileEntry, 0, len(dirs)+len(files))
	for _, d := range dirs {
		out = append(out, d.entry)
	}
	out = append(out, files...)
	return out, nil
}

func (s *Server) GetSharePage(w http.ResponseWriter, r *http.Request) {
	share, target, err := s.resolveShare(r)
	if err != nil {
		if isBot(r) {
			s.writeOG(w, r, ogPrivate())
			return
		}
		status := http.StatusNotFound
		if errors.Is(err, errShareExpired) {
			status = http.StatusGone
		} else if err.Error() == "forbidden" {
			status = http.StatusForbidden
		}
		s.writeError(w, r, err, status)
		return
	}
	if !underShare(share.Path, target) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}

	if isBot(r) {
		if file, err := s.store.Q.GetFile(r.Context(), target); err == nil {
			s.writeOG(w, r, ogPublic(file.Path, file.Description, file.ContentType, file.Size))
		} else {
			s.writeOG(w, r, ogPublic(target, "", "", 0))
		}
		return
	}

	file, fileErr := s.store.Q.GetFile(r.Context(), target)
	sharePerms, err := s.shareEffectivePerms(r.Context(), share.ID, target)
	if err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}

	if fileErr == nil && !storage.IsDirectory(file.ContentType) {
		if !sharePerms.Has(acl.PermissionRead) {
			s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
			return
		}
		if r.URL.Query().Get("preview") == "1" {
			s.serveImagePreview(w, r, target)
			return
		}
		s.streamFile(w, r, target, file.ContentType, wantsDownload(r))
		return
	}
	if wantsDownload(r) {
		if !sharePerms.Has(acl.PermissionRead) {
			s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
			return
		}
		s.streamZip(w, r, target, GetUserInfo(r), true)
		return
	}

	if wantsJSON(r) {
		if !sharePerms.Has(acl.PermissionRead) {
			s.writeJSON(w, map[string]any{
				"path":        shareBrowsePath(share, target),
				"share_id":    share.ID,
				"files":       []FileEntry{},
				"permissions": uint64(sharePerms),
			}, http.StatusOK)
			return
		}
		entries, err := s.listShareDir(r.Context(), share, target, sharePerms)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "forbidden" {
				status = http.StatusForbidden
			}
			s.writeError(w, r, err, status)
			return
		}
		s.writeJSON(w, map[string]any{
			"path":        shareBrowsePath(share, target),
			"share_id":    share.ID,
			"files":       entries,
			"permissions": uint64(sharePerms),
		}, http.StatusOK)
		return
	}
	s.serveSPA(w, r)
}

func (s *Server) GetSharePreview(w http.ResponseWriter, r *http.Request) {
	share, target, err := s.resolveShare(r)
	if err != nil || !underShare(share.Path, target) {
		status := http.StatusNotFound
		if errors.Is(err, errShareExpired) {
			status = http.StatusGone
		}
		http.Error(w, http.StatusText(status), status)
		return
	}
	s.serveImagePreview(w, r, target)
}

func (s *Server) ShareUploadFileAPI(w http.ResponseWriter, r *http.Request) {
	share, targetDir, err := s.resolveShare(r)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, errShareExpired) {
			status = http.StatusGone
		}
		s.writeError(w, r, err, status)
		return
	}
	perms, err := s.shareEffectivePerms(r.Context(), share.ID, targetDir)
	if err != nil || !perms.Has(acl.PermissionCreate) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		s.writeError(w, r, errors.New("mkdir via share not supported yet"), http.StatusNotImplemented)
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
		Replace     bool   `json:"replace"`
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
			_ = json.NewDecoder(part).Decode(&meta)
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
	ident := &acl.Identity{ShareID: share.ID}
	target, errs := s.validateUploadTarget(r.Context(), targetDir, meta.Name, meta.Size, meta.Replace, nil, ident, share.Path)
	if len(errs) > 0 {
		s.writeError(w, r, errors.New(errs[0]), http.StatusBadRequest)
		return
	}
	if meta.Size > s.cfg.Upload.ChunkSize.Bytes {
		s.writeError(w, r, errors.New("use resumable upload for large files"), http.StatusBadRequest)
		return
	}
	ct := sniffContentType(path.Base(target), filePart.Header.Get("Content-Type"))
	_ = s.storage.Mkdir(r.Context(), path.Dir(target))
	if err := s.storage.PutObject(r.Context(), target, meta.Size, filePart, ct); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	params := dbq.UpsertFileParams{
		Path: target, Size: meta.Size, ContentType: ct, Description: meta.Description,
		UserID: database.NullString(&share.UserID), CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.store.Q.UpsertFile(r.Context(), params); err != nil {
		s.writeError(w, r, err, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
