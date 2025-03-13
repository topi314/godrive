package godrive

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/topi314/godrive/godrive/auth"
	"github.com/topi314/godrive/godrive/database"
	"github.com/topi314/godrive/godrive/storage"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type (
	ExecuteTemplateFunc func(w io.Writer, name string, data any) error
	WriterFunc          func(w io.Writer) error
)

type (
	ErrorResponse struct {
		Message   string `json:"message"`
		Status    int    `json:"status"`
		Path      string `json:"path"`
		RequestID string `json:"request_id"`
	}

	WarningResponse struct {
		Message   string `json:"message"`
		Status    int    `json:"status"`
		Path      string `json:"path"`
		RequestID string `json:"request_id"`
	}
)

func NewServer(version string, cfg Config, db *database.DB, auth *auth.Auth, storage storage.Storage, tracer trace.Tracer, meter metric.Meter, assets http.FileSystem) *Server {
	s := &Server{
		version: version,
		cfg:     cfg,
		db:      db,
		auth:    auth,
		storage: storage,
		tracer:  tracer,
		meter:   meter,
		assets:  assets,
	}

	s.server = &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: s.Routes(),
	}

	return s
}

type Server struct {
	version string
	cfg     Config
	db      *database.DB
	server  *http.Server
	auth    *auth.Auth
	storage storage.Storage
	tracer  trace.Tracer
	meter   metric.Meter
	assets  http.FileSystem
}

func (s *Server) Start() {
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("Error while listening", slog.Any("err", err))
	}
}

func (s *Server) Close() {
	if err := s.server.Close(); err != nil {
		slog.Error("Error while closing server", slog.Any("err", err))
	}

	if err := s.db.Close(); err != nil {
		slog.Error("Error while closing database", slog.Any("err", err))
	}
}

func FormatBuildVersion(version string, commit string, buildTime time.Time) string {
	if len(commit) > 7 {
		commit = commit[:7]
	}

	buildTimeStr := "unknown"
	if !buildTime.IsZero() {
		buildTimeStr = buildTime.Format(time.ANSIC)
	}
	return fmt.Sprintf("Go Version: %s\nVersion: %s\nCommit: %s\nBuild Time: %s\nOS/Arch: %s/%s\n", runtime.Version(), version, commit, buildTimeStr, runtime.GOOS, runtime.GOARCH)
}
