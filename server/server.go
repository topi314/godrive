package server

import (
	"context"
	"crypto/rand"
	"fmt"
	"io/fs"
	"log/slog"
	"math/big"
	"net/http"
	"sync"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/topi314/godrive/frontend"
	"github.com/topi314/godrive/server/config"
	"github.com/topi314/godrive/server/database"
	"github.com/topi314/godrive/server/oidc"
	"github.com/topi314/godrive/server/storage"
	"golang.org/x/oauth2"
)

type Server struct {
	version   string
	cfg       config.Config
	store     *database.Store
	auth      *oidc.Client
	storage   storage.Storage
	public    fs.FS
	cancel    context.CancelFunc
	http      *http.Server
	refreshMu sync.Mutex
}

// New constructs the server (DB, storage, auth, sync, frontend FS).
func New(cfg config.Config, version string) (*Server, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := database.NewStore(ctx, cfg.Database, database.Migrations)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}

	st, err := storage.New(context.Background(), cfg.Storage)
	if err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("storage: %w", err)
	}

	var auth *oidc.Client
	if cfg.Auth != nil {
		provider, err := gooidc.NewProvider(context.Background(), cfg.Auth.Issuer)
		if err != nil {
			_ = st.Close()
			_ = store.Close()
			return nil, fmt.Errorf("oidc: %w", err)
		}
		auth = &oidc.Client{
			Provider: provider,
			Verifier: provider.Verifier(&gooidc.Config{ClientID: cfg.Auth.ClientID}),
			Config: &oauth2.Config{
				ClientID:     cfg.Auth.ClientID,
				ClientSecret: cfg.Auth.ClientSecret,
				Endpoint:     provider.Endpoint(),
				RedirectURL:  cfg.Auth.RedirectURL,
				Scopes:       []string{gooidc.ScopeOpenID, "groups", "email", "profile", gooidc.ScopeOfflineAccess},
			},
			States: map[string]oidc.LoginFlow{},
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
		storage: st,
		public:  publicFS,
		cancel:  runCancel,
	}
	if s.auth != nil {
		endSession := ""
		if cfg.Auth != nil {
			endSession = cfg.Auth.EndSessionEndpoint
		}
		s.auth.LoadDiscovery(endSession)
	}
	if err := s.seedDefaultRootACL(ctx); err != nil {
		runCancel()
		_ = st.Close()
		_ = store.Close()
		return nil, fmt.Errorf("seed default ACL: %w", err)
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
