package main

//go:generate sqlc generate

import (
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/topi314/godrive/server"
)

var (
	Version   = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config file")
	flag.Parse()

	cfg, err := server.LoadConfig(*cfgPath)
	if err != nil {
		slog.Error("Error while loading config", slog.Any("err", err))
		os.Exit(1)
	}
	setupLogger(cfg.Log)

	buildTime, _ := time.Parse(time.RFC3339, BuildTime)
	version := server.FormatBuildVersion(Version, Commit, buildTime)
	slog.Info("Starting godrive...",
		slog.String("version", Version),
		slog.String("commit", Commit),
		slog.Time("build_time", buildTime),
	)

	srv, err := server.New(cfg, version)
	if err != nil {
		slog.Error("Error while creating server", slog.Any("err", err))
		os.Exit(1)
	}

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server failed", slog.Any("err", err))
			os.Exit(1)
		}
	}()
	defer srv.Stop()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
}

func setupLogger(cfg server.LogConfig) {
	opts := &slog.HandlerOptions{AddSource: cfg.AddSource, Level: cfg.Level}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
