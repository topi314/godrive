package godrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/riandyrn/otelchi"
	"github.com/samber/slog-chi"
	"github.com/topi314/godrive/godrive/auth"
	"github.com/topi314/godrive/godrive/database"
	"github.com/topi314/godrive/templates"
)

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(otelchi.Middleware("gobin", otelchi.WithChiRoutes(r)))
	r.Use(middleware.Maybe(middleware.StripSlashes, func(r *http.Request) bool {
		return !strings.HasPrefix(r.URL.Path, "/debug/")
	}))
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(slogchi.NewWithConfig(slog.Default(), slogchi.Config{
		DefaultLevel:     slog.LevelInfo,
		ClientErrorLevel: slog.LevelDebug,
		ServerErrorLevel: slog.LevelError,
		WithRequestID:    true,
		WithRequestBody:  true,
		WithResponseBody: true,
		WithSpanID:       s.cfg.Otel != nil,
		WithTraceID:      s.cfg.Otel != nil,
		Filters: []slogchi.Filter{
			slogchi.IgnorePathPrefix("/assets"),
		},
	}))
	r.Use(cacheControl)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))

	if s.cfg.Debug {
		r.Mount("/debug", middleware.Profiler())
	}

	r.Group(func(r chi.Router) {
		if s.cfg.DevMode {
			r.Mount("/assets", http.StripPrefix("/assets", http.FileServer(s.assets)))
			r.Handle("/favicon.ico", s.serveAsset("/favicon.png"))
			r.Handle("/favicon.png", s.serveAsset("/favicon.png"))
			r.Handle("/favicon-light.png", s.serveAsset("/favicon-light.png"))
			r.Handle("/robots.txt", s.serveAsset("/robots.txt"))
		} else {
			r.Use(replacePrefix("/assets", "/public"))
			r.Mount("/assets", http.FileServer(s.assets))
			r.Handle("/favicon.ico", s.serveAsset("/public/favicon.png"))
			r.Handle("/favicon.png", s.serveAsset("/public/favicon.png"))
			r.Handle("/favicon-light.png", s.serveAsset("/public/favicon-light.png"))
			r.Handle("/robots.txt", s.serveAsset("/public/robots.txt"))
		}
	})

	r.Get("/version", s.GetVersion)

	r.Group(func(r chi.Router) {
		if s.cfg.Auth != nil {
			r.Use(s.Auth)
			r.Route("/api", func(r chi.Router) {
				r.Get("/login", s.Login)
				r.Get("/callback", s.Callback)
				r.Get("/logout", s.Logout)
				r.Route("/settings", func(r chi.Router) {
					r.Get("/", s.GetSettings)
					r.Head("/", s.GetSettings)
					r.Patch("/", s.PatchSettings)
				})
			})
		}

		r.Group(func(r chi.Router) {
			if s.cfg.Auth != nil {
				r.Use(s.CheckAuth(func(r *http.Request, info *auth.UserInfo) auth.Action {
					if s.auth.HasAccess(info) {
						return auth.ActionAllow
					}
					if r.Method == http.MethodGet {
						return auth.ActionLogin
					}
					return auth.ActionDeny
				}))
			}
			r.Get("/share/{shareID}", s.GetShare)
			r.Get("/share/{shareID}/*", s.GetShare)
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

func (s *Server) GetVersion(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(s.version))
}

func (s *Server) handleWriter(wf WriterFunc, mediaType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", mediaType)
		if err := wf(w); err != nil {
			slog.ErrorContext(r.Context(), "error writing response", slog.Any("err", err))
		}
	})
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if err := templates.NotFound(s.pageVars(r)).Render(r.Context(), w); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
	}
}

func (s *Server) error(w http.ResponseWriter, r *http.Request, err error, status int) {
	if errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "internal server error", slog.Any("err", err))
	}
	if r.Header.Get("Accept") == "text/html" {
		w.WriteHeader(status)
		if err = templates.ErrorRs(err).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "failed to execute error rs template", slog.Any("err", err))
		}
		return
	}
	s.json(w, r, ErrorResponse{
		Message:   err.Error(),
		Status:    status,
		Path:      r.URL.Path,
		RequestID: middleware.GetReqID(r.Context()),
	}, status)
}

func (s *Server) warn(w http.ResponseWriter, r *http.Request, message string, status int) {
	s.json(w, r, WarningResponse{
		Message:   message,
		Status:    status,
		Path:      r.URL.Path,
		RequestID: middleware.GetReqID(r.Context()),
	}, status)
}

func (s *Server) prettyError(w http.ResponseWriter, r *http.Request, err error, status int) {
	if status == http.StatusInternalServerError {
		slog.ErrorContext(r.Context(), "internal server error", slog.Any("err", err))
	}
	w.WriteHeader(status)

	vars := templates.ErrorVars{
		Error:     err,
		Status:    status,
		Path:      r.URL.Path,
		RequestID: middleware.GetReqID(r.Context()),
	}
	if tmplErr := templates.Error(vars, s.pageVars(r)); tmplErr != nil {
		slog.ErrorContext(r.Context(), "failed to execute error template", slog.Any("err", tmplErr))
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

	if err := json.NewEncoder(w).Encode(v); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		slog.ErrorContext(r.Context(), "failed to encode json", slog.Any("err", err))
	}
}

func (s *Server) pageVars(r *http.Request) templates.PageVars {
	return templates.PageVars{
		Theme:    "dark",
		HashAuth: s.cfg.Auth != nil,
		URL:      fmt.Sprintf("%s%s", s.cfg.PublicURL, r.URL.String()),
		User:     s.newTemplateUser(auth.GetUserInfo(r)),
	}
}

func (s *Server) userType(groups []string) templates.UserType {
	if s.auth.IsAdmin(groups) {
		return templates.UserTypeAdmin
	} else if s.auth.IsUser(groups) {
		return templates.UserTypeUser
	} else if s.auth.IsGuest(groups) {
		return templates.UserTypeGuest
	}
	return templates.UserTypeNone
}

func (s *Server) newTemplateUser(info *auth.UserInfo) templates.User {
	return templates.User{
		ID:     info.Subject,
		Name:   info.Username,
		Groups: info.Groups,
		Email:  info.Email,
		Home:   info.Home,
		Type:   s.userType(info.Groups),
	}
}

func (s *Server) newTemplateFile(file database.File, perms auth.Permissions, dbPerms []database.Permissions) templates.File {
	owner := "Unknown"
	if file.Username != nil {
		owner = *file.Username
	}
	date := file.CreatedAt
	if file.UpdatedAt.After(date) {
		date = file.UpdatedAt
	}

	var filePerms []templates.Permissions
	for _, perm := range dbPerms {
		name := perm.Object
		if perm.ObjectName != nil {
			name = *perm.ObjectName
		}
		filePerms = append(filePerms, templates.Permissions{
			Path:       perm.Path,
			ObjectType: auth.ObjectType(perm.ObjectType),
			Object:     perm.Object,
			ObjectName: name,
			Allow:      auth.Permissions(perm.Allow),
			Deny:       auth.Permissions(perm.Deny),
			Map:        mapPermissions(auth.Permissions(perm.Allow), auth.Permissions(perm.Deny)),
		})
	}

	return templates.File{
		IsDir:           false,
		Path:            file.Path,
		Name:            path.Base(file.Path),
		Dir:             path.Dir(file.Path),
		Size:            file.Size,
		Description:     file.Description,
		Date:            date,
		Owners:          []string{owner},
		OwnerIDs:        []string{file.UserID},
		Permissions:     perms,
		FilePermissions: filePerms,
	}
}
