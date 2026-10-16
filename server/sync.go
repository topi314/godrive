package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/storage"
)

func (s *Server) startSync(ctx context.Context) {
	_ = s.storage.Watch(ctx, func(ev storage.Event) {
		s.applyStorageEvent(context.Background(), ev)
	})

	interval := time.Duration(0)
	if s.cfg.Storage.SyncInterval != nil {
		interval = s.cfg.Storage.SyncInterval.Duration
	}
	if interval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := s.reconcileAll(ctx); err != nil {
					slog.Warn("storage reconcile failed", slog.Any("err", err))
				}
			}
		}
	}()
}

func (s *Server) applyStorageEvent(ctx context.Context, ev storage.Event) {
	path := acl.NormalizePath(ev.Path)
	if acl.IsReservedPath(path) {
		return
	}
	switch ev.Type {
	case storage.EventDelete:
		_ = s.store.Q.DeleteFile(ctx, path)
	case storage.EventUpsert:
		info := ev.Info
		if info.Path == "" {
			var err error
			info, err = s.storage.Stat(ctx, path)
			if err != nil {
				return
			}
		}
		info.Path = path
		s.upsertIndexedFile(ctx, info)
	}
}

func (s *Server) syncPrefix(ctx context.Context, prefix string) {
	prefix = acl.NormalizePath(prefix)
	// GetPath syncs the requested path before streaming. S3 List uses prefix+"/",
	// which misses a file object itself — the reconcile pass would then delete it
	// from the index and the new-tab open would fall through to the SPA/redirect.
	if prefix != "/" {
		if info, err := s.storage.Stat(ctx, prefix); err == nil && !storage.IsDirectory(info.ContentType) {
			s.upsertIndexedFile(ctx, info)
			return
		}
	}

	objs, err := s.storage.List(ctx, prefix)
	if err != nil {
		return
	}
	seen := map[string]struct{}{}
	for _, obj := range objs {
		if acl.IsReservedPath(obj.Path) {
			continue
		}
		markSeenWithAncestors(seen, obj.Path)
		s.upsertIndexedFile(ctx, obj)
	}
	rows, err := s.store.Q.ListFilesUnder(ctx, database.ListFilesUnderParams{
		Path: prefix, PathLike: acl.LikeUnder(prefix),
	})
	if err != nil {
		return
	}
	for _, row := range rows {
		if _, ok := seen[row.Path]; !ok {
			_ = s.store.Q.DeleteFile(ctx, row.Path)
		}
	}
}

func markSeenWithAncestors(seen map[string]struct{}, p string) {
	p = acl.NormalizePath(p)
	for p != "/" {
		seen[p] = struct{}{}
		parent := path.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
}

// upsertIndexedFile indexes a storage object. Size is the change signal: list/sync
// often returns a different MIME than the DB, and GetPath runs sync on every refresh,
// so comparing content-type (or always writing UpdatedAt=now) would rewrite stamps.
func (s *Server) upsertIndexedFile(ctx context.Context, info storage.ObjectInfo) {
	p := acl.NormalizePath(info.Path)
	if p == "" || acl.IsReservedPath(p) {
		return
	}
	stamp := time.Now().UTC()
	if !info.LastModified.IsZero() {
		stamp = info.LastModified.UTC()
	}
	ct := info.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}

	existing, err := s.store.Q.GetFile(ctx, p)
	if err == nil {
		if existing.Size != info.Size {
			if existing.ContentType != "" {
				ct = existing.ContentType
			}
			_, _ = s.store.Q.UpsertFile(ctx, database.UpsertFileParams{
				Path: p, Size: info.Size, ContentType: ct,
				Description: existing.Description, UserID: existing.UserID,
				CreatedAt: existing.CreatedAt, UpdatedAt: stamp,
			})
		}
	} else {
		_, _ = s.store.Q.UpsertFile(ctx, database.UpsertFileParams{
			Path: p, Size: info.Size, ContentType: ct,
			CreatedAt: stamp, UpdatedAt: stamp,
		})
	}
	// S3 lists often omit directory placeholders; keep parent folders in the index.
	s.ensureIndexedParents(ctx, p, stamp)
}

// ensureIndexedParents inserts missing directory index rows (and storage markers) for
// each ancestor of filePath so nested paths stay browsable as folders.
func (s *Server) ensureIndexedParents(ctx context.Context, filePath string, stamp time.Time) {
	for d := path.Dir(acl.NormalizePath(filePath)); d != "/" && d != "."; d = path.Dir(d) {
		if acl.IsReservedPath(d) {
			break
		}
		if _, err := s.store.Q.GetFile(ctx, d); err == nil {
			continue
		}
		_ = s.storage.Mkdir(ctx, d)
		_, _ = s.store.Q.UpsertFile(ctx, dirFileParams(d, stamp, ""))
	}
}

func (s *Server) reconcileAll(ctx context.Context) error {
	s.syncPrefix(ctx, "/")
	return nil
}

func (s *Server) StorageEventsWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.storageWebhookAuthorized(r) {
		s.writeError(w, r, errors.New("unauthorized"), http.StatusUnauthorized)
		return
	}
	var body struct {
		Records []struct {
			EventName string `json:"eventName"`
			S3        struct {
				Object struct {
					Key string `json:"key"`
				} `json:"object"`
			} `json:"s3"`
		} `json:"Records"`
		Key string `json:"Key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, err, http.StatusBadRequest)
		return
	}
	for _, rec := range body.Records {
		key := acl.NormalizePath("/" + rec.S3.Object.Key)
		if strings.Contains(rec.EventName, "ObjectRemoved") {
			s.applyStorageEvent(r.Context(), storage.Event{Type: storage.EventDelete, Path: key})
		} else {
			s.applyStorageEvent(r.Context(), storage.Event{Type: storage.EventUpsert, Path: key})
		}
	}
	if body.Key != "" {
		s.applyStorageEvent(r.Context(), storage.Event{Type: storage.EventUpsert, Path: acl.NormalizePath("/" + body.Key)})
	}
	w.WriteHeader(http.StatusNoContent)
}

// storageWebhookAuthorized matches MinIO notify_webhook auth_token:
// Authorization is compared to "Bearer <auth_token>" (or the full auth_token if it already includes a scheme).
func (s *Server) storageWebhookAuthorized(r *http.Request) bool {
	token := strings.TrimSpace(s.cfg.Storage.S3.Notify.AuthToken)
	if token == "" {
		return true
	}
	want := token
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		want = "Bearer " + token
	}
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
