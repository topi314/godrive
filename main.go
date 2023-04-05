package main

import (
	"context"
	"embed"
	"flag"
	"github.com/dustin/go-humanize"
	"github.com/topisenpai/godrive/godrive"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
)

// These variables are set via the -ldflags option in go build
var (
	version   = "unknown"
	commit    = "unknown"
	buildTime = "unknown"
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
	log.Printf("Starting godrive with version: %s (commit: %s, build time: %s)...", version, commit, buildTime)
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
		log.Fatalln("Error while reading config:", err)
	}
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetEnvPrefix("godrive")
	viper.AutomaticEnv()

	var cfg godrive.Config
	if err := viper.Unmarshal(&cfg, func(config *mapstructure.DecoderConfig) {
		config.TagName = "cfg"
	}); err != nil {
		log.Fatalln("Error while unmarshalling config:", err)
	}
	log.Println("Config:", cfg)

	if cfg.Debug {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := godrive.NewDB(ctx, cfg.Database, Schema)
	if err != nil {
		log.Fatalln("Error while connecting to database:", err)
	}
	defer db.Close()

	key := jose.SigningKey{
		Algorithm: jose.HS512,
		Key:       []byte(cfg.JWTSecret),
	}
	signer, err := jose.NewSigner(key, nil)
	if err != nil {
		log.Fatalln("Error while creating signer:", err)
	}

	storage, err := godrive.NewStorage(context.Background(), cfg.Storage)
	if err != nil {
		log.Fatalln("Error while creating storage:", err)
	}

	var (
		tmplFunc godrive.ExecuteTemplateFunc
		assets   http.FileSystem
	)

	funcs := template.FuncMap{
		"humanizeTime":  humanize.Time,
		"humanizeBytes": humanize.Bytes,
		"isLast": func(slice any, index int) bool {
			return reflect.ValueOf(slice).Len()-1 == index
		},
		"assemblePath": func(slice []string, index int) string {
			return strings.Join(slice[:index+1], "/")
		},
	}

	if cfg.DevMode {
		log.Println("Development mode enabled")
		tmplFunc = func(wr io.Writer, name string, data any) error {
			tmpl := template.New("").Funcs(funcs)
			tmpl = template.Must(tmpl.ParseGlob("templates/*"))
			return tmpl.ExecuteTemplate(wr, name, data)
		}
		assets = http.Dir(".")
	} else {
		tmpl := template.New("").Funcs(funcs)
		tmpl = template.Must(tmpl.ParseFS(Templates, "templates/*"))
		tmplFunc = tmpl.ExecuteTemplate
		assets = http.FS(Assets)
	}

	s := godrive.NewServer(godrive.FormatBuildVersion(version, commit, buildTime), cfg, db, signer, storage, assets, tmplFunc)
	log.Println("godrive listening on:", cfg.ListenAddr)
	go s.Start()
	defer s.Close()

	si := make(chan os.Signal, 1)
	signal.Notify(si, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-si
}
