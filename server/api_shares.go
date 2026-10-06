package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/topi314/godrive/server/database/dbsqlc"
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
	if IsReservedPath(p) {
		s.writeError(w, r, errors.New("reserved path"), http.StatusBadRequest)
		return
	}
	perms, err := s.EffectivePermissions(r.Context(), p, info, nil)
	if err != nil || (s.cfg.Auth != nil && !perms.Has(PermissionShare)) {
		s.writeError(w, r, errors.New("forbidden"), http.StatusForbidden)
		return
	}
	id := s.newShareID()
	now := time.Now().UTC()
	share, err := s.store.Q.CreateShare(r.Context(), dbsqlc.CreateShareParams{
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

// underShare reports whether target is exactly root or a descendant of root.
func underShare(root, target string) bool {
	root = NormalizePath(root)
	target = NormalizePath(target)
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
	root := NormalizePath(shareRoot)
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
		target = NormalizePath(path.Join(append([]string{root}, parts...)...))
	}
	if !underShare(root, target) {
		return "", errors.New("forbidden")
	}
	return target, nil
}

func (s *Server) resolveShare(r *http.Request) (dbsqlc.Share, string, error) {
	id := chi.URLParam(r, "id")
	if id == "" || strings.Contains(id, "/") || strings.Contains(id, "..") {
		return dbsqlc.Share{}, "", errors.New("not found")
	}
	share, err := s.store.Q.GetShare(r.Context(), id)
	if err != nil {
		return share, "", err
	}
	if share.ExpiresAt.Valid && time.Now().After(share.ExpiresAt.Time) {
		return share, "", errors.New("expired")
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
func shareBrowsePath(share dbsqlc.Share, storagePath string) string {
	root := NormalizePath(share.Path)
	storagePath = NormalizePath(storagePath)
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

// listShareDir lists immediate children under target with the share as content root.
// Paths in the result are /s/{id}/… browse URLs; ACL is not applied (share grants read).
func (s *Server) listShareDir(ctx context.Context, share dbsqlc.Share, target string) ([]FileEntry, error) {
	target = NormalizePath(target)
	if !underShare(share.Path, target) {
		return nil, errors.New("forbidden")
	}
	s.syncPrefix(ctx, target)

	rows, err := s.store.Q.ListFilesUnder(ctx, dbsqlc.ListFilesUnderParams{
		Path:     target,
		PathLike: LikeUnder(target),
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

		if row.Path == target {
			// Shared path is itself a file — surface as a single entry at share root.
			entry := s.fileEntryFromRow(ctx, row, owner, PermissionRead)
			entry.Path = shareBrowsePath(share, row.Path)
			files = append(files, entry)
			continue
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(row.Path, target), "/")
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
			dpath := NormalizePath(path.Join(target, name))
			dirs[name] = &agg{entry: FileEntry{
				Path: shareBrowsePath(share, dpath), Name: name, IsDir: true, Size: row.Size,
				Date: updated, Permissions: uint64(PermissionRead),
			}}
			continue
		}
		entry := s.fileEntryFromRow(ctx, row, owner, PermissionRead)
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
		if err.Error() == "forbidden" {
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
	if fileErr == nil {
		if r.URL.Query().Get("preview") == "1" {
			s.serveImagePreview(w, r, target)
			return
		}
		// Real files always stream (inline or attachment); SPA is only for directories.
		s.streamFile(w, r, target, file.ContentType, wantsDownload(r))
		return
	}

	if wantsJSON(r) {
		entries, err := s.listShareDir(r.Context(), share, target)
		if err != nil {
			status := http.StatusInternalServerError
			if err.Error() == "forbidden" {
				status = http.StatusForbidden
			}
			s.writeError(w, r, err, status)
			return
		}
		s.writeJSON(w, map[string]any{
			"path":     shareBrowsePath(share, target),
			"share_id": share.ID,
			"files":    entries,
		}, http.StatusOK)
		return
	}
	s.serveSPA(w, r)
}

func (s *Server) GetSharePreview(w http.ResponseWriter, r *http.Request) {
	share, target, err := s.resolveShare(r)
	if err != nil || !underShare(share.Path, target) {
		http.NotFound(w, r)
		return
	}
	s.serveImagePreview(w, r, target)
}
