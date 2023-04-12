package godrive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type (
	TemplateVariables struct {
		Path      string
		PathParts []string
		Files     []TemplateFile
		Theme     string
		Auth      bool
		User      *TemplateUser
	}

	TemplateUser struct {
		Name  string
		Email string
	}

	TemplateFile struct {
		IsDir       bool
		Dir         string
		Name        string
		Size        uint64
		Description string
		Private     bool
		Date        time.Time
		Owner       string
	}

	FileRequest struct {
		Size        uint64 `json:"size"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}

	ErrorResponse struct {
		Message   string `json:"message"`
		Status    int    `json:"status"`
		Path      string `json:"path"`
		RequestID string `json:"request_id"`
	}
)

func (f TemplateFile) FullName() string {
	return path.Join(f.Dir, f.Name)
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.CleanPath)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Maybe(
		middleware.RequestLogger(&middleware.DefaultLogFormatter{
			Logger: log.Default(),
		}),
		func(r *http.Request) bool {
			// Don't log requests for assets
			return !strings.HasPrefix(r.URL.Path, "/assets")
		},
	))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))
	if s.cfg.Auth != nil {
		r.Use(s.Auth)
	}

	if s.cfg.Debug {
		r.Mount("/debug", middleware.Profiler())
	}

	r.Route("/assets", func(r chi.Router) {
		r.Handle("/script.js", s.handleWriter(s.js, "application/javascript"))
		r.Handle("/style.css", s.handleWriter(s.css, "text/css"))
		r.Mount("/", http.FileServer(s.assets))
	})
	r.Handle("/favicon.ico", s.file("/assets/favicon.png"))
	r.Handle("/favicon.png", s.file("/assets/favicon.png"))
	r.Handle("/favicon-light.png", s.file("/assets/favicon-light.png"))
	r.Handle("/robots.txt", s.file("/assets/robots.txt"))

	r.Get("/version", s.GetVersion)

	if s.cfg.Auth != nil {
		r.Group(func(r chi.Router) {
			r.Get("/login", s.Login)
			r.Get("/callback", s.Callback)
			r.Get("/logout", s.Logout)
			r.Route("/settings", func(r chi.Router) {
				//r.Get("/", s.GetSettings)
				//r.Head("/", s.GetSettings)
				//r.Patch("/", s.PatchSettings)
			})
		})
	}
	r.Group(func(r chi.Router) {
		if s.cfg.Auth != nil {
			r.Use(s.CheckAuth(func(r *http.Request, info *UserInfo) AuthAction {
				if s.hasAccess(info) {
					return AuthActionAllow
				}
				if r.Method == http.MethodGet {
					return AuthActionLogin
				}
				return AuthActionDeny
			}))
		}
		r.Get("/*", s.GetFiles)
		r.Head("/*", s.GetFiles)
		r.Post("/*", s.PostFiles)
		r.Patch("/*", s.PatchFiles)
		r.Delete("/*", s.DeleteFiles)

	})
	r.NotFound(s.notFound)

	return r
}

func (s *Server) handleWriter(wf WriterFunc, mediatype string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", mediatype)
		if err := wf(w); err != nil {
			s.log(r, "writer", err)
		}
	})
}

func (s *Server) GetVersion(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(s.version))
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if err := s.tmpl(w, "404.gohtml", nil); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
	}
}

func (s *Server) log(r *http.Request, logType string, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		return
	}
	log.Printf("Error while handling %s(%s) %s: %s\n", logType, middleware.GetReqID(r.Context()), r.RequestURI, err)
}

func (s *Server) error(w http.ResponseWriter, r *http.Request, err error, status int) {
	if errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
	if status == http.StatusInternalServerError {
		s.log(r, "request", err)
	}
	s.json(w, r, ErrorResponse{
		Message:   err.Error(),
		Status:    status,
		Path:      r.URL.Path,
		RequestID: middleware.GetReqID(r.Context()),
	}, status)
}

func (s *Server) prettyError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if status == http.StatusInternalServerError {
		s.log(r, "pretty request", err)
	}
	w.WriteHeader(status)

	vars := map[string]any{
		"Error":     err.Error(),
		"Status":    status,
		"RequestID": middleware.GetReqID(r.Context()),
		"Path":      r.URL.Path,
	}
	if tmplErr := s.tmpl(w, "error.gohtml", vars); tmplErr != nil {
		s.log(r, "template", tmplErr)
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

	if err := json.NewEncoder(w).Encode(v); err != nil && err != http.ErrHandlerTimeout {
		s.log(r, "json", err)
	}
}

func (s *Server) file(path string) http.HandlerFunc {
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
