package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/topi314/godrive/server/database/dbsqlc"
)

func (s *Server) startSync(ctx context.Context) {
	_ = s.storage.Watch(ctx, func(ev StorageEvent) {
		s.applyStorageEvent(context.Background(), ev)
	})

	interval := s.cfg.Storage.SyncInterval.Duration
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

func (s *Server) applyStorageEvent(ctx context.Context, ev StorageEvent) {
	path := NormalizePath(ev.Path)
	if IsReservedPath(path) {
		return
	}
	switch ev.Type {
	case StorageEventDelete:
		_ = s.store.Q.DeleteFile(ctx, path)
		_ = s.store.Q.DeleteACLForPath(ctx, path)
		_ = s.store.Q.DeleteSharesForPath(ctx, path)
	case StorageEventUpsert:
		info := ev.Info
		if info.Path == "" {
			var err error
			info, err = s.storage.Stat(ctx, path)
			if err != nil {
				return
			}
		}
		now := time.Now().UTC()
		ct := info.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		_, _ = s.store.Q.UpsertFile(ctx, dbsqlc.UpsertFileParams{
			Path: path, Size: info.Size, ContentType: ct,
			Description: "", CreatedAt: now, UpdatedAt: now,
		})
	}
}

func (s *Server) syncPrefix(ctx context.Context, prefix string) {
	objs, err := s.storage.List(ctx, prefix)
	if err != nil {
		return
	}
	seen := map[string]struct{}{}
	now := time.Now().UTC()
	for _, obj := range objs {
		if IsReservedPath(obj.Path) {
			continue
		}
		seen[obj.Path] = struct{}{}
		ct := obj.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		_, _ = s.store.Q.UpsertFile(ctx, dbsqlc.UpsertFileParams{
			Path: obj.Path, Size: obj.Size, ContentType: ct,
			Description: "", CreatedAt: now, UpdatedAt: now,
		})
	}
	rows, err := s.store.Q.ListFilesUnder(ctx, dbsqlc.ListFilesUnderParams{
		Path: NormalizePath(prefix), PathLike: LikeUnder(prefix),
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

func (s *Server) reconcileAll(ctx context.Context) error {
	s.syncPrefix(ctx, "/")
	return nil
}

func (s *Server) StorageEventsWebhook(w http.ResponseWriter, r *http.Request) {
	secret := s.cfg.Storage.Notify.WebhookSecret
	if secret != "" && r.Header.Get("X-Godrive-Secret") != secret {
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
		key := NormalizePath("/" + rec.S3.Object.Key)
		if strings.Contains(rec.EventName, "ObjectRemoved") {
			s.applyStorageEvent(r.Context(), StorageEvent{Type: StorageEventDelete, Path: key})
		} else {
			s.applyStorageEvent(r.Context(), StorageEvent{Type: StorageEventUpsert, Path: key})
		}
	}
	if body.Key != "" {
		s.applyStorageEvent(r.Context(), StorageEvent{Type: StorageEventUpsert, Path: NormalizePath("/" + body.Key)})
	}
	w.WriteHeader(http.StatusNoContent)
}
