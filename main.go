package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"embed"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/dustin/go-humanize"
	"github.com/topi314/godrive/godrive"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/exp/slog"
	"golang.org/x/oauth2"

	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// These variables are set via the -ldflags option in go build
var (
	Name      = "godrive"
	Namespace = "github.com/topi314/godrive"

	Version   = "unknown"
	Commit    = "unknown"
	BuildTime = "unknown"
)

var (
	//go:embed templates
	Templates embed.FS

	//go:embed assets
	Assets embed.FS

	//go:embed sql/schema.sql
	Schema string
)

func main() {
	cfgPath := flag.String("config", "", "path to godrive.json")
	flag.Parse()

	viper.SetDefault("listen_addr", ":80")
	viper.SetDefault("dev_mode", false)
	viper.SetDefault("debug", false)
	viper.SetDefault("database_type", "sqlite")
	viper.SetDefault("database_debug", false)
	viper.SetDefault("database_expire_after", "0")
	viper.SetDefault("database_cleanup_interval", "1m")
	viper.SetDefault("database_path", "gobin.db")
	viper.SetDefault("database_host", "localhost")
	viper.SetDefault("database_port", 5432)
	viper.SetDefault("database_username", "gobin")
	viper.SetDefault("database_database", "gobin")
	viper.SetDefault("database_ssl_mode", "disable")
	viper.SetDefault("storage_type", "local")
	viper.SetDefault("storage_path", "/etc/godrive/storage")

	if *cfgPath != "" {
		viper.SetConfigFile(*cfgPath)
	} else {
		viper.SetConfigName("godrive")
		viper.SetConfigType("json")
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/godrive/")
	}
	if err := viper.ReadInConfig(); err != nil {
		slog.Error("Error while reading config", slog.Any("err", err))
		os.Exit(-1)
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("godrive")
	viper.AutomaticEnv()

	var cfg godrive.Config
	if err := viper.Unmarshal(&cfg, func(config *mapstructure.DecoderConfig) {
		config.TagName = "cfg"
	}); err != nil {
		slog.Error("Error while unmarshalling config", slog.Any("err", err))
		os.Exit(-1)
	}
	setupLogger(cfg.Log)
	buildTime, _ := time.Parse(time.RFC3339, BuildTime)
	slog.Info("Starting godrive", slog.Any("version", Version), slog.Any("commit", Commit), slog.Any("buildTime", buildTime), slog.Any("config", cfg))

	var (
		tracer trace.Tracer
		meter  metric.Meter
		err    error
	)
	if cfg.Otel != nil {
		tracer, err = newTracer(*cfg.Otel)
		if err != nil {
			slog.Error("Error while creating tracer", slog.Any("err", err))
			os.Exit(1)
		}
		meter, err = newMeter(*cfg.Otel)
		if err != nil {
			slog.Error("Error while creating meter", slog.Any("err", err))
			os.Exit(1)
		}
	} else {
		tracer = trace.NewNoopTracerProvider().Tracer(Namespace)
	}

	var auth *godrive.Auth
	if cfg.Auth != nil {
		provider, err := oidc.NewProvider(context.Background(), cfg.Auth.Issuer)
		if err != nil {
			slog.Error("Error while creating oidc provider", slog.Any("err", err))
			os.Exit(-1)
		}

		auth = &godrive.Auth{
			Provider: provider,
			Verifier: provider.Verifier(&oidc.Config{
				ClientID: cfg.Auth.ClientID,
			}),
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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := godrive.NewDB(ctx, cfg.Database, Schema)
	if err != nil {
		slog.Error("Error while connecting to database", slog.Any("err", err))
		os.Exit(-1)
	}
	defer db.Close()

	storage, err := godrive.NewStorage(context.Background(), cfg.Storage, tracer)
	if err != nil {
		slog.Error("Error while creating storage", slog.Any("err", err))
		os.Exit(-1)
	}

	funcs := template.FuncMap{
		"humanizeTime":   humanize.Time,
		"humanizeIBytes": humanize.IBytes,
		"isLast": func(slice any, index int) bool {
			return reflect.ValueOf(slice).Len()-1 == index
		},
		"assemblePath": func(slice []string, index int) string {
			return strings.Join(slice[:index+1], "/")
		},
		"gravatarURL": func(email string) string {
			return fmt.Sprintf("https://www.gravatar.com/avatar/%x?s=%d&d=retro", md5.Sum([]byte(strings.ToLower(email))), 80)
		},
	}

	var (
		tmplFunc godrive.ExecuteTemplateFunc
		cssFunc  godrive.WriterFunc
		assets   http.FileSystem
	)
	if cfg.DevMode {
		slog.Info("Running in dev mode")
		tmplFunc = func(wr io.Writer, name string, data any) error {
			tmpl := template.New("").Funcs(funcs)
			tmpl = template.Must(tmpl.ParseGlob("templates/*.gohtml"))
			tmpl = template.Must(tmpl.ParseGlob("templates/dialogs/*.gohtml"))
			return tmpl.ExecuteTemplate(wr, name, data)
		}
		cssFunc = writeDir(os.DirFS("assets"), "css/*")
		assets = http.Dir(".")
	} else {
		tmpl := template.New("").Funcs(funcs)
		tmpl = template.Must(tmpl.ParseFS(Templates, "templates/*.gohtml"))
		tmpl = template.Must(tmpl.ParseFS(Templates, "templates/dialogs/*.gohtml"))
		tmplFunc = tmpl.ExecuteTemplate

		jsBuff := new(bytes.Buffer)
		if err = writeDir(Assets, "assets/js/*")(jsBuff); err != nil {
			slog.Error("Error while minifying js", slog.Any("err", err))
		}

		cssBuff := new(bytes.Buffer)
		if err = writeDir(Assets, "assets/css/*")(cssBuff); err != nil {
			slog.Error("Error while minifying css", slog.Any("err", err))
		}

		cssFunc = func(w io.Writer) error {
			_, err = w.Write(cssBuff.Bytes())
			return err
		}
		assets = http.FS(Assets)
	}

	s := godrive.NewServer(godrive.FormatBuildVersion(Version, Commit, buildTime), cfg, db, auth, storage, tracer, meter, assets, tmplFunc, cssFunc)
	slog.Info("godrive listening", slog.String("listen_addr", cfg.ListenAddr))
	go s.Start()
	defer s.Close()

	si := make(chan os.Signal, 1)
	signal.Notify(si, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-si
}

func writeDir(fs fs.FS, pattern string) func(w io.Writer) error {
	return func(w io.Writer) error {
		fr, err := newFolderReader(fs, pattern)
		if err != nil {
			return err
		}
		defer fr.Close()
		_, err = io.Copy(w, fr)
		return err
	}
}

func setupLogger(cfg godrive.LogConfig) {
	opts := &slog.HandlerOptions{
		AddSource: cfg.AddSource,
		Level:     cfg.Level,
	}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
