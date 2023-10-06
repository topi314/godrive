package godrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/topi314/godrive/templates"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type (
	ExecuteTemplateFunc func(w io.Writer, name string, data any) error
	WriterFunc          func(w io.Writer) error
)

func NewServer(version string, cfg Config, db *DB, auth *Auth, storage Storage, tracer trace.Tracer, meter metric.Meter, assets http.FileSystem) *Server {
	s := &Server{
		version: version,
		cfg:     cfg,
		db:      db,
		auth:    auth,
		storage: storage,
		tracer:  tracer,
		meter:   meter,
		assets:  assets,
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
	rand    *rand.Rand
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

func (s *Server) newTemplateUser(info *UserInfo) templates.User {
	return templates.User{
		ID:      info.Subject,
		Name:    info.Username,
		Email:   info.Email,
		Home:    info.Home,
		IsAdmin: s.isAdmin(info),
		IsUser:  s.isUser(info),
		IsGuest: s.isGuest(info),
	}
}

func (s *Server) newTemplateFile(userInfo *UserInfo, file File) templates.File {
	owner := "Unknown"
	if file.Username != nil {
		owner = *file.Username
	}
	date := file.CreatedAt
	if file.UpdatedAt.After(date) {
		date = file.UpdatedAt
	}
	return templates.File{
		IsDir:       false,
		Path:        file.Path,
		Name:        path.Base(file.Path),
		Dir:         path.Dir(file.Path),
		Size:        file.Size,
		Description: file.Description,
		Date:        date,
		Owner:       owner,
		IsOwner:     file.UserID == userInfo.Subject || s.isAdmin(userInfo),
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

func (s *Server) handleWriter(wf WriterFunc, mediaType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", mediaType)
		if err := wf(w); err != nil {
			slog.ErrorContext(r.Context(), "error writing response", slog.Any("err", err))
		}
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if err := templates.NotFound("dark", false, templates.User{}).Render(r.Context(), w); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
	}
}

func (s *Server) error(w http.ResponseWriter, r *http.Request, err error, status int) {
	if errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "internal server error", slog.Any("err", err))
	}
	if r.Header.Get("Accept") == "text/html" {
		w.WriteHeader(status)
		if err = templates.ErrorRs(err).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "failed to execute error rs template", slog.Any("err", err))
		}
		return
	}
	s.json(w, r, ErrorResponse{
		Message:   err.Error(),
		Status:    status,
		Path:      r.URL.Path,
		RequestID: middleware.GetReqID(r.Context()),
	}, status)
}

func (s *Server) warn(w http.ResponseWriter, r *http.Request, message string, status int) {
	s.json(w, r, WarningResponse{
		Message:   message,
		Status:    status,
		Path:      r.URL.Path,
		RequestID: middleware.GetReqID(r.Context()),
	}, status)
}

func (s *Server) prettyError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "internal server error", slog.Any("err", err))
	}
	w.WriteHeader(status)

	vars := templates.ErrorVars{
		Error:     err,
		Status:    status,
		Path:      r.URL.Path,
		RequestID: middleware.GetReqID(r.Context()),
	}

	if tmplErr := templates.Error("dark", false, templates.User{}, vars); tmplErr != nil {
		slog.ErrorContext(r.Context(), "failed to execute error template", slog.Any("err", tmplErr))
	}
}

func (s *Server) ok(w http.ResponseWriter, r *http.Request, v any) {
	s.json(w, r, v, http.StatusOK)
}

func (s *Server) json(w http.ResponseWriter, r *http.Request, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}

	if err := json.NewEncoder(w).Encode(v); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		slog.ErrorContext(r.Context(), "failed to encode json", slog.Any("err", err))
	}
}

func (s *Server) serveAsset(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file, err := s.assets.Open(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer file.Close()
		_, _ = io.Copy(w, file)
	}
}

func ReplacePrefix(prefix string, replace string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, prefix) {
			h.ServeHTTP(w, r)
			return
		}
		p := replace + strings.TrimPrefix(r.URL.Path, prefix)
		rp := replace + strings.TrimPrefix(r.URL.RawPath, prefix)
		r2 := new(http.Request)
		*r2 = *r
		r2.URL = new(url.URL)
		*r2.URL = *r.URL
		r2.URL.Path = p
		r2.URL.RawPath = rp
		h.ServeHTTP(w, r2)
	})
}
