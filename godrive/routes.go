package godrive

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riandyrn/otelchi"
	"github.com/topi314/godrive/internal/log"
	"github.com/topi314/godrive/templates"
)

type (
	FileCreateRequest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Overwrite   bool   `json:"overwrite"`
		Size        uint64 `json:"size"`
	}

	FileUpdateRequest struct {
		Dir         string `json:"dir"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Size        uint64 `json:"size"`
	}

	ErrorResponse struct {
		Message   string `json:"message"`
		Status    int    `json:"status"`
		Path      string `json:"path"`
		RequestID string `json:"request_id"`
	}

	WarningResponse struct {
		Message   string `json:"message"`
		Status    int    `json:"status"`
		Path      string `json:"path"`
		RequestID string `json:"request_id"`
	}
)

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(otelchi.Middleware("gobin", otelchi.WithChiRoutes(r)))
	r.Use(middleware.CleanPath)
	r.Use(middleware.StripSlashes)
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Maybe(
		log.StructuredLogger,
		func(r *http.Request) bool {
			// Don't log requests for assets
			return !strings.HasPrefix(r.URL.Path, "/assets")
		},
	))
	r.Use(cacheControl)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))

	if s.cfg.Debug {
		r.Mount("/debug", middleware.Profiler())
	}

	if s.cfg.DevMode {
		r.Mount("/assets", http.StripPrefix("/assets", http.FileServer(s.assets)))
	} else {
		r.Mount("/assets", ReplacePrefix("/assets", "/public", http.FileServer(s.assets)))
	}
	r.Handle("/favicon.ico", s.serveAsset("/assets/favicon.png"))
	r.Handle("/favicon.png", s.serveAsset("/assets/favicon.png"))
	r.Handle("/favicon-light.png", s.serveAsset("/assets/favicon-light.png"))
	r.Handle("/robots.txt", s.serveAsset("/assets/robots.txt"))

	r.Get("/version", s.GetVersion)

	r.Group(func(r chi.Router) {
		if s.cfg.Auth != nil {
			r.Use(s.Auth)
			r.Group(func(r chi.Router) {
				r.Get("/login", s.Login)
				r.Get("/callback", s.Callback)
				r.Get("/logout", s.Logout)
				r.Route("/settings", func(r chi.Router) {
					r.Get("/", s.GetSettings)
					// r.Head("/", s.GetSettings)
					// r.Patch("/", s.PatchSettings)
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
			r.Post("/*", s.PostFile)
			r.Patch("/*", s.PatchFile)
			r.Delete("/*", s.DeleteFiles)
		})
	})
	r.NotFound(s.notFound)

	return r
}

func (s *Server) GetSettings(w http.ResponseWriter, r *http.Request) {
	userInfo := GetUserInfo(r)
	if !s.isAdmin(userInfo) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	users, err := s.db.GetAllUsers(r.Context())
	if err != nil {
		s.prettyError(w, r, err, http.StatusInternalServerError)
		return
	}

	templateUsers := make([]templates.User, len(users))
	for i, user := range users {
		templateUsers[i] = templates.User{
			ID:    user.ID,
			Name:  user.Username,
			Email: user.Email,
			Home:  user.Home,
		}
	}

	w.Write([]byte("TODO"))
}

func (s *Server) GetVersion(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(s.version))
}
