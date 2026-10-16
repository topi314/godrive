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

	"github.com/topi314/godrive/server"
	"github.com/topi314/godrive/server/config"
)

func main() {
	cfgPath := flag.String("config", "config.toml", "path to config file")
	flag.Parse()

	cfg, err := config.LoadConfig(*cfgPath)
	if err != nil {
		slog.Error("Error while loading config", slog.Any("err", err))
		os.Exit(1)
	}
	setupLogger(cfg.Log)

	bi := server.ReadBuildInfo()
	slog.Info("Starting godrive...",
		slog.String("version", bi.Version),
		slog.String("commit", bi.Commit),
		slog.Time("build_time", bi.BuildTime),
		slog.Bool("dirty", bi.Modified),
		slog.Any("config", cfg),
	)

	srv, err := server.New(cfg, bi.Format())
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

func setupLogger(cfg config.LogConfig) {
	opts := &slog.HandlerOptions{AddSource: cfg.AddSource, Level: cfg.Level}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
