package godrive

import (
	"context"
	"io/fs"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Heartbeat("/ping"))
	r.Use(s.AuthMiddleware)

	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		s.writeJSON(w, map[string]string{"version": s.version}, http.StatusOK)
	})

	if s.public != nil {
		publicFS := s.public
		if sub, err := fs.Sub(s.public, "public"); err == nil {
			publicFS = sub
		}
		fileServer := http.FileServer(http.FS(publicFS))
		r.Handle("/_nuxt/*", http.StripPrefix("/", fileServer))
		r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, publicFS, "favicon.ico")
		})
		r.Get("/favicon.png", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, publicFS, "favicon.png")
		})
		r.Get("/og-card.png", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, publicFS, "og-card.png")
		})
		r.Get("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, publicFS, "robots.txt")
		})
	}

	r.Route("/api", func(r chi.Router) {
		r.Get("/me", s.Me)
		if s.cfg.Auth != nil {
			r.Get("/login", s.Login)
			r.Get("/callback", s.Callback)
			r.Post("/logout", s.Logout)
			r.Get("/logout", s.Logout)
		}

		r.Post("/internal/storage-events", s.StorageEventsWebhook)

		r.Group(func(r chi.Router) {
			r.Use(s.RequireAccess)
			r.Get("/files", s.ListFilesAPI)
			r.Get("/files/*", s.GetFileAPI)
			r.Post("/files/*", s.UploadFileAPI)
			r.Patch("/files/*", s.PatchFileAPI)
			r.Put("/files/*", s.MoveFilesAPI)
			r.Delete("/files/*", s.DeleteFilesAPI)

			r.Get("/permissions", s.GetPermissionsAPI)
			r.Put("/permissions", s.PutPermissionsAPI)

			r.Get("/shares", s.ListSharesAPI)
			r.Post("/shares", s.CreateShareAPI)
			r.Delete("/shares/{id}", s.DeleteShareAPI)

			r.Get("/tokens", s.ListTokensAPI)
			r.Post("/tokens", s.CreateTokenAPI)
			r.Delete("/tokens/{hash}", s.DeleteTokenAPI)

			r.Get("/settings/users", s.ListUsersAPI)
			r.Get("/settings/permissions", s.ListAllPermissionsAPI)
		})
	})

	r.Route("/s/{id}", func(r chi.Router) {
		r.Get("/preview", s.GetSharePreview)
		r.Get("/", s.GetSharePage)
		r.Get("/*", s.GetSharePage)
	})

	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if GetUserInfo(r) == nil && s.cfg.Auth != nil && s.cfg.Auth.Groups.Guest {
					info := &UserInfo{Subject: "guest", Username: "guest", Groups: []string{"guest"}, Home: "/"}
					r = r.WithContext(context.WithValue(r.Context(), UserInfoKey, info))
				}
				next.ServeHTTP(w, r)
			})
		})
		r.Get("/*", s.GetPath)
		r.Head("/*", s.GetPath)
		r.With(s.RequireAccess).Post("/*", s.UploadFileAPI)
		r.With(s.RequireAccess).Patch("/*", s.PatchFileAPI)
		r.With(s.RequireAccess).Put("/*", s.MoveFilesAPI)
		r.With(s.RequireAccess).Delete("/*", s.DeleteFilesAPI)
	})

	return r
}
