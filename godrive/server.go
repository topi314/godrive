package godrive

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"net/http"
	"time"
)

type Server struct {
	version string
	cfg     Config
	store   *Store
	auth    *Auth
	storage Storage
	public  fs.FS
	cancel  context.CancelFunc
}

func NewServer(version string, cfg Config, store *Store, auth *Auth, storage Storage, publicFS fs.FS) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		version: version,
		cfg:     cfg,
		store:   store,
		auth:    auth,
		storage: storage,
		public:  publicFS,
		cancel:  cancel,
	}
	s.startSync(ctx)
	return s
}

func (s *Server) Start() error {
	srv := &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("godrive listening", slog.String("addr", s.cfg.ListenAddr))
	return srv.ListenAndServe()
}

func (s *Server) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.storage != nil {
		_ = s.storage.Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error, status int) {
	slog.ErrorContext(r.Context(), "request error", slog.String("path", r.URL.Path), slog.Any("err", err))
	s.writeJSON(w, map[string]any{
		"message": err.Error(),
		"status":  status,
		"path":    r.URL.Path,
	}, status)
}

func FormatBuildVersion(version, commit string, buildTime time.Time) string {
	if buildTime.IsZero() {
		return fmt.Sprintf("%s (%s)", version, commit)
	}
	return fmt.Sprintf("%s (%s) built %s", version, commit, buildTime.Format(time.RFC3339))
}

func (s *Server) newShareID() string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 12)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		b[i] = letters[n.Int64()]
	}
	return string(b)
}
