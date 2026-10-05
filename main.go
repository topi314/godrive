package main

import (
	"context"
	"embed"
	"flag"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/topi314/godrive/godrive"
	"golang.org/x/oauth2"
)

var (
	Name      = "godrive"
	Namespace = "github.com/topi314/godrive"

	Version   = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

//go:embed migrations
var Migrations embed.FS

//go:embed all:public
var Public embed.FS

func main() {
	cfgPath := flag.String("config", "", "path to godrive.toml")
	flag.Parse()

	cfg, err := godrive.LoadConfig(*cfgPath)
	if err != nil {
		slog.Error("config error", slog.Any("err", err))
		os.Exit(1)
	}
	setupLogger(cfg.Log)

	buildTime, _ := time.Parse(time.RFC3339, BuildTime)
	slog.Info("starting godrive",
		slog.String("version", Version),
		slog.String("commit", Commit),
		slog.Time("buildTime", buildTime),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := godrive.NewStore(ctx, cfg.Database, Migrations)
	if err != nil {
		slog.Error("database error", slog.Any("err", err))
		os.Exit(1)
	}

	storage, err := godrive.NewStorage(context.Background(), cfg.Storage)
	if err != nil {
		slog.Error("storage error", slog.Any("err", err))
		os.Exit(1)
	}

	var auth *godrive.Auth
	if cfg.Auth != nil {
		provider, err := oidc.NewProvider(context.Background(), cfg.Auth.Issuer)
		if err != nil {
			slog.Error("oidc error", slog.Any("err", err))
			os.Exit(1)
		}
		auth = &godrive.Auth{
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

	var publicFS fs.FS = Public
	if cfg.DevMode {
		if _, err := os.Stat("public"); err == nil {
			publicFS = os.DirFS(".")
		}
	}

	version := godrive.FormatBuildVersion(Version, Commit, buildTime)
	s := godrive.NewServer(version, cfg, store, auth, storage, publicFS)

	go func() {
		if err := s.Start(); err != nil {
			slog.Error("server stopped", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	s.Close()
}

func setupLogger(cfg godrive.LogConfig) {
	opts := &slog.HandlerOptions{AddSource: cfg.AddSource, Level: cfg.Level}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
