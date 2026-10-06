package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(s.AuthMiddleware)

	if s.public != nil {
		fileServer := http.FileServer(http.FS(s.public))
		r.Handle("/_nuxt/*", http.StripPrefix("/", fileServer))
		r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, s.public, "favicon.ico")
		})
		r.Get("/favicon.png", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, s.public, "favicon.png")
		})
		r.Get("/favicon-light.png", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, s.public, "favicon-light.png")
		})
		r.Get("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, s.public, "robots.txt")
		})
	}

	// Legacy OIDC redirect (pre-/api rewrite). Keep for existing IdP client configs.
	if s.cfg.Auth != nil {
		r.Get("/callback", s.Callback)
	}

	r.Route("/api", func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("."))
		})
		r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
			s.writeJSON(w, map[string]string{"version": s.version}, http.StatusOK)
		})
		if s.public != nil {
			r.Get("/og-card.png", func(w http.ResponseWriter, r *http.Request) {
				http.ServeFileFS(w, r, s.public, "og-card.png")
			})
		}

		r.Get("/me", s.Me)
		r.Get("/login", s.Login)
		r.Post("/refresh", s.Refresh)
		r.Get("/logout", s.Logout)
		r.Post("/logout", s.Logout)
		if s.cfg.Auth != nil {
			r.Get("/callback", s.Callback)
		}

		r.Get("/upload/config", s.UploadConfigAPI)
		// Share-authorized upload sessions (anonymous OK when share grants Create).
		r.Post("/uploads/preflight", s.PreflightUploadAPI)
		r.Post("/uploads", s.CreateUploadSessionAPI)
		r.Get("/uploads/{id}", s.GetUploadSessionAPI)
		r.Patch("/uploads/{id}", s.PatchUploadSessionAPI)
		r.Post("/uploads/{id}/complete", s.CompleteUploadSessionAPI)
		r.Delete("/uploads/{id}", s.AbortUploadSessionAPI)

		r.Post("/internal/storage-events", s.StorageEventsWebhook)

		r.Group(func(r chi.Router) {
			r.Use(s.RequireAccess)

			r.Get("/permissions", s.GetPermissionsAPI)
			r.Put("/permissions", s.PutPermissionsAPI)

			r.Get("/shares", s.ListSharesAPI)
			r.Post("/shares", s.CreateShareAPI)
			r.Delete("/shares/{id}", s.DeleteShareAPI)

			r.Get("/tokens", s.ListTokensAPI)
			r.Post("/tokens", s.CreateTokenAPI)
			r.Delete("/tokens/{hash}", s.DeleteTokenAPI)

			r.Patch("/me", s.PatchMeAPI)

			r.Get("/settings/users", s.ListUsersAPI)
			r.Patch("/settings/users/{id}", s.PatchUserAPI)
			r.Delete("/settings/users/{id}", s.DeleteUserAPI)
			r.Get("/settings/permissions", s.ListAllPermissionsAPI)
		})
	})

	r.Route("/s/{id}", func(r chi.Router) {
		r.Get("/preview", s.GetSharePreview)
		r.Get("/", s.GetSharePage)
		r.Get("/*", s.GetSharePage)
		// Mutating share routes (authorized via share ACL principal).
		r.Post("/", s.ShareUploadFileAPI)
		r.Post("/*", s.ShareUploadFileAPI)
	})

	// Path-based file API (content negotiation): JSON list vs bytes/HTML via Accept.
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
		r.Get("/", s.GetPath)
		r.Get("/*", s.GetPath)
		r.Head("/", s.GetPath)
		r.Head("/*", s.GetPath)
		r.With(s.RequireAccess).Post("/", s.UploadFileAPI)
		r.With(s.RequireAccess).Post("/*", s.UploadFileAPI)
		r.With(s.RequireAccess).Patch("/", s.PatchFileAPI)
		r.With(s.RequireAccess).Patch("/*", s.PatchFileAPI)
		r.With(s.RequireAccess).Put("/", s.MoveFilesAPI)
		r.With(s.RequireAccess).Put("/*", s.MoveFilesAPI)
		r.With(s.RequireAccess).Delete("/", s.DeleteFilesAPI)
		r.With(s.RequireAccess).Delete("/*", s.DeleteFilesAPI)
	})

	return r
}
