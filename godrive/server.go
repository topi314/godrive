package godrive

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"runtime"
	"time"
)

type (
	ExecuteTemplateFunc func(w io.Writer, name string, data any) error
	WriterFunc          func(w io.Writer) error
)

func NewServer(version string, cfg Config, db *DB, auth *Auth, storage Storage, assets http.FileSystem, tmpl ExecuteTemplateFunc, js WriterFunc, css WriterFunc) *Server {
	s := &Server{
		version: version,
		cfg:     cfg,
		db:      db,
		auth:    auth,
		storage: storage,
		assets:  assets,
		tmpl:    tmpl,
		js:      js,
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
	assets  http.FileSystem
	tmpl    ExecuteTemplateFunc
	js      WriterFunc
	css     WriterFunc
	rand    *rand.Rand
}

func (s *Server) Start() {
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalln("Error while listening:", err)
	}
}

func (s *Server) Close() {
	if err := s.server.Close(); err != nil {
		log.Println("Error while closing server:", err)
	}

	if err := s.db.Close(); err != nil {
		log.Println("Error while closing database:", err)
	}
}

func (s *Server) newID() string {
	b := make([]rune, 16)
	for i := range b {
		b[i] = letters[s.rand.Intn(len(letters))]
	}
	return string(b)
}

func FormatBuildVersion(version string, commit string, buildTime string) string {
	if len(commit) > 7 {
		commit = commit[:7]
	}

	buildTimeStr := "unknown"
	if buildTime != "unknown" {
		parsedTime, _ := time.Parse(time.RFC3339, buildTime)
		if !parsedTime.IsZero() {
			buildTimeStr = parsedTime.Format(time.ANSIC)
		}
	}
	return fmt.Sprintf("Go Version: %s\nVersion: %s\nCommit: %s\nBuild Time: %s\nOS/Arch: %s/%s\n", runtime.Version(), version, commit, buildTimeStr, runtime.GOOS, runtime.GOARCH)
}
