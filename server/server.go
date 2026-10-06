package server

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

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/topi314/godrive/frontend"
	"github.com/topi314/godrive/server/database"
	"golang.org/x/oauth2"
)

type Server struct {
	version string
	cfg     Config
	store   *Store
	auth    *Auth
	storage Storage
	public  fs.FS
	cancel  context.CancelFunc
	http    *http.Server
}

// New constructs the server (DB, storage, auth, sync, frontend FS).
func New(cfg Config, version string) (*Server, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := NewStore(ctx, cfg.Database, database.Migrations)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	storage, err := NewStorage(context.Background(), cfg.Storage)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("storage: %w", err)
	}

	var auth *Auth
	if cfg.Auth != nil {
		provider, err := oidc.NewProvider(context.Background(), cfg.Auth.Issuer)
		if err != nil {
			_ = storage.Close()
			_ = store.Close()
			return nil, fmt.Errorf("oidc: %w", err)
		}
		auth = &Auth{
			Provider: provider,
			Verifier: provider.Verifier(&oidc.Config{ClientID: cfg.Auth.ClientID}),
			Config: &oauth2.Config{
				ClientID:     cfg.Auth.ClientID,
				ClientSecret: cfg.Auth.ClientSecret,
				Endpoint:     provider.Endpoint(),
				RedirectURL:  cfg.Auth.RedirectURL,
				Scopes:       []string{oidc.ScopeOpenID, "groups", "email", "profile", oidc.ScopeOfflineAccess},
			},
			States: map[string]string{},
		}
	}

	var publicFS fs.FS
	if dist, err := frontend.Dist(); err == nil {
		publicFS = dist
	}

	runCtx, runCancel := context.WithCancel(context.Background())
	s := &Server{
		version: version,
		cfg:     cfg,
		store:   store,
		auth:    auth,
		storage: storage,
		public:  publicFS,
		cancel:  runCancel,
	}
	s.startSync(runCtx)
	return s, nil
}

func (s *Server) Start() error {
	s.http = &http.Server{
		Addr:              s.cfg.ListenAddr,
		Handler:           s.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("godrive listening", slog.String("addr", s.cfg.ListenAddr))
	return s.http.ListenAndServe()
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.http.Shutdown(ctx)
	}
	if s.storage != nil {
		_ = s.storage.Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
}

// Close is an alias for Stop for compatibility.
func (s *Server) Close() { s.Stop() }

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
