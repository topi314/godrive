package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
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
	objs, err := s.storage.List(ctx, prefix)
	if err != nil {
		return
	}
	seen := map[string]struct{}{}
	for _, obj := range objs {
		if acl.IsReservedPath(obj.Path) {
			continue
		}
		seen[obj.Path] = struct{}{}
		s.upsertIndexedFile(ctx, obj)
	}
	rows, err := s.store.Q.ListFilesUnder(ctx, database.ListFilesUnderParams{
		Path: acl.NormalizePath(prefix), PathLike: acl.LikeUnder(prefix),
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

// upsertIndexedFile indexes a storage object. Size is the change signal: list/sync
// often returns a different MIME than the DB, and GetPath runs sync on every refresh,
// so comparing content-type (or always writing UpdatedAt=now) would rewrite stamps.
func (s *Server) upsertIndexedFile(ctx context.Context, info storage.ObjectInfo) {
	path := acl.NormalizePath(info.Path)
	if path == "" || acl.IsReservedPath(path) {
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

	existing, err := s.store.Q.GetFile(ctx, path)
	if err == nil {
		if existing.Size == info.Size {
			return
		}
		if existing.ContentType != "" {
			ct = existing.ContentType
		}
		_, _ = s.store.Q.UpsertFile(ctx, database.UpsertFileParams{
			Path: path, Size: info.Size, ContentType: ct,
			Description: existing.Description, UserID: existing.UserID,
			CreatedAt: existing.CreatedAt, UpdatedAt: stamp,
		})
		return
	}
	_, _ = s.store.Q.UpsertFile(ctx, database.UpsertFileParams{
		Path: path, Size: info.Size, ContentType: ct,
		CreatedAt: stamp, UpdatedAt: stamp,
	})
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
