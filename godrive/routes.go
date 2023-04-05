package godrive

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"golang.org/x/exp/slices"
)

const maxUnix = int(^int32(0))

var (
	ErrRateLimit = errors.New("rate limit exceeded")
)

type (
	TemplateVariables struct {
		Path      string
		PathParts []string
		Files     []File
		Theme     string
	}

	FileRequest struct {
		Path        string `json:"path"`
		Size        int64  `json:"size"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
	}

	FileResponse struct {
		Path        string    `json:"path"`
		Name        string    `json:"name"`
		Size        int64     `json:"size"`
		ContentType string    `json:"content_type"`
		Description string    `json:"description"`
		Private     bool      `json:"private"`
		CreatedAt   time.Time `json:"created_at"`
		UpdatedAt   time.Time `json:"updated_at"`
	}

	ErrorResponse struct {
		Message   string `json:"message"`
		Status    int    `json:"status"`
		Path      string `json:"path"`
		RequestID string `json:"request_id"`
	}
)

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.CleanPath)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Compress(5))
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
	if s.cfg.RateLimit != nil {
		r.Use(s.RateLimit)
	}
	r.Use(s.JWTMiddleware)

	if s.cfg.Debug {
		r.Mount("/debug", middleware.Profiler())
	}

	r.Mount("/assets", http.FileServer(s.assets))
	r.Handle("/favicon.ico", s.file("/assets/favicon.png"))
	r.Handle("/favicon.png", s.file("/assets/favicon.png"))
	r.Handle("/favicon-light.png", s.file("/assets/favicon-light.png"))
	r.Handle("/robots.txt", s.file("/assets/robots.txt"))
	r.Group(func(r chi.Router) {
		r.Route("/api", func(r chi.Router) {
			r.Route("/files", func(r chi.Router) {
				r.Post("/", s.PostFile)
				//r.Get("/", s.GetFiles)
				r.Route("/{id}", func(r chi.Router) {
					//r.Get("/", s.GetFile)
					//r.Head("/", s.GetFile)
					//r.Patch("/", s.PatchFile)
					//r.Delete("/", s.DeleteFile)
				})
			})
		})
		r.Get("/version", s.GetVersion)
		r.Get("/*", s.GetHome)
		r.Head("/*", s.GetHome)
	})
	r.NotFound(s.notFound)

	if s.cfg.HTTPTimeout > 0 {
		return http.TimeoutHandler(r, s.cfg.HTTPTimeout, "Request timed out")
	}
	return r
}

func (s *Server) GetHome(w http.ResponseWriter, r *http.Request) {
	files, err := s.db.GetFiles(r.Context(), r.URL.Path)
	if err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	vars := TemplateVariables{
		Path:      r.URL.Path,
		PathParts: strings.FieldsFunc(r.URL.Path, func(r rune) bool { return r == '/' }),
		Files:     files,
		Theme:     "dark",
	}
	if err = s.tmpl(w, "index.gohtml", vars); err != nil {
		log.Println("failed to execute template:", err)
	}
}

func (s *Server) PostFile(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		s.error(w, r, err, http.StatusBadRequest)
		return
	}

	part, err := mr.NextPart()
	if err != nil {
		s.error(w, r, err, http.StatusBadRequest)
		return
	}
	defer part.Close()

	if part.FormName() != "json" {
		s.error(w, r, errors.New("json field not found"), http.StatusBadRequest)
		return
	}

	var file FileRequest
	if err = json.NewDecoder(part).Decode(&file); err != nil {
		s.error(w, r, err, http.StatusBadRequest)
		return
	}

	part, err = mr.NextPart()
	if err != nil {
		s.error(w, r, err, http.StatusBadRequest)
		return
	}
	defer part.Close()

	if part.FormName() != "file" {
		s.error(w, r, errors.New("file field not found"), http.StatusBadRequest)
		return
	}
	fileName := part.FileName()
	if file.Path == "" {
		file.Path = "/"
	}
	contentType := part.Header.Get("Content-Type")

	createdFile, err := s.db.CreateFile(r.Context(), file.Path, fileName, file.Size, contentType, file.Description, file.Private)
	if err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	if err = s.storage.PutObject(r.Context(), path.Join(file.Path, fileName), file.Size, part, contentType); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	s.ok(w, r, FileResponse{
		Path:        createdFile.Path,
		Name:        createdFile.Name,
		Size:        createdFile.Size,
		ContentType: createdFile.ContentType,
		Description: createdFile.Description,
		Private:     createdFile.Private,
		CreatedAt:   createdFile.CreatedAt,
		UpdatedAt:   createdFile.UpdatedAt,
	})
}

func (s *Server) GetVersion(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(s.version))
}

func (s *Server) redirectRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if err := s.tmpl(w, "404.gohtml", nil); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
	}
}

func (s *Server) rateLimit(w http.ResponseWriter, r *http.Request) {
	s.error(w, r, ErrRateLimit, http.StatusTooManyRequests)
}

func (s *Server) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only apply rate limiting to POST, PATCH, and DELETE requests
		if r.Method != http.MethodPost && r.Method != http.MethodPatch && r.Method != http.MethodDelete {
			next.ServeHTTP(w, r)
			return
		}
		remoteAddr := strings.SplitN(r.RemoteAddr, ":", 2)[0]
		// Filter whitelisted IPs
		if slices.Contains(s.cfg.RateLimit.Whitelist, remoteAddr) {
			next.ServeHTTP(w, r)
			return
		}
		// Filter blacklisted IPs
		if slices.Contains(s.cfg.RateLimit.Blacklist, remoteAddr) {
			retryAfter := maxUnix - int(time.Now().Unix())
			w.Header().Set("X-RateLimit-Limit", "0")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(maxUnix))
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			s.rateLimit(w, r)
			return
		}
		if s.rateLimitHandler == nil {
			next.ServeHTTP(w, r)
			return
		}
		s.rateLimitHandler(next).ServeHTTP(w, r)
	})
}

func (s *Server) log(r *http.Request, logType string, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		return
	}
	log.Printf("Error while handling %s(%s) %s: %s\n", logType, middleware.GetReqID(r.Context()), r.RequestURI, err)
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
	if tmplErr := s.tmpl(w, "error.gohtml", vars); tmplErr != nil && tmplErr != http.ErrHandlerTimeout {
		s.log(r, "template", tmplErr)
	}
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
