package godrive

import (
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"runtime"
	"strings"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
)

type (
	ExecuteTemplateFunc func(w io.Writer, name string, data any) error
	WriterFunc          func(w io.Writer) error
)

func NewServer(version string, cfg Config, db *DB, auth *Auth, storage Storage, tracer trace.Tracer, meter metric.Meter, assets http.FileSystem, tmpl ExecuteTemplateFunc, css WriterFunc) *Server {
	s := &Server{
		version: version,
		cfg:     cfg,
		db:      db,
		auth:    auth,
		storage: storage,
		tracer:  tracer,
		meter:   meter,
		assets:  assets,
		tmpl:    tmpl,
		css:     css,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
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
	db      *DB
	server  *http.Server
	auth    *Auth
	storage Storage
	tracer  trace.Tracer
	meter   metric.Meter
	assets  http.FileSystem
	tmpl    ExecuteTemplateFunc
	css     WriterFunc
	rand    *rand.Rand
}

func (s *Server) Start() {
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func (s *Server) newID(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letters[s.rand.Intn(len(letters))]
	}
	return string(b)
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=86400")
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		next.ServeHTTP(w, r)
	})
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
