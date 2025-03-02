package godrive

import (
	"log/slog"
	"net/http"

	"github.com/topi314/godrive/godrive/auth"
	"github.com/topi314/godrive/templates"
)

func (s *Server) GetSettings(w http.ResponseWriter, r *http.Request) {
	userInfo := auth.GetUserInfo(r)
	if !s.auth.IsAdmin(userInfo.Groups) {
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
			ID:     user.ID,
			Name:   user.Username,
			Groups: user.Groups,
			Email:  user.Email,
			Home:   user.Home,
			Type:   s.userType(user.Groups),
		}
	}

	permissions, err := s.db.GetAllPermissions(r.Context())
	templatePermissions := make([]templates.Permissions, len(permissions))
	for i, permission := range permissions {
		objectName := auth.ObjectType(permission.ObjectType).String()
		if auth.ObjectType(permission.ObjectType) == auth.ObjectTypeUser {
			for _, user := range users {
				if user.ID == permission.Object {
					objectName = user.Username
					break
				}
			}
		}

		templatePermissions[i] = templates.Permissions{
			Path:       permission.Path,
			ObjectType: auth.ObjectType(permission.ObjectType),
			Object:     permission.Object,
			ObjectName: objectName,
			Allow:      auth.Permissions(permission.Allow),
			Deny:       auth.Permissions(permission.Deny),
			Map:        mapPermissions(auth.Permissions(permission.Allow), auth.Permissions(permission.Deny)),
		}
	}

	vars := templates.SettingsVars{
		Users:       templateUsers,
		Permissions: templatePermissions,
	}
	if err = templates.Settings(vars, s.pageVars(r)).Render(r.Context(), w); err != nil {
		slog.ErrorContext(r.Context(), "error executing template", slog.Any("err", err))
		return
	}
}

func mapPermissions(allow auth.Permissions, deny auth.Permissions) map[string]templates.ToggleState {
	perms := make(map[string]templates.ToggleState)
	for perm, name := range auth.AllPermissions {
		if allow.Has(perm) {
			perms[name] = templates.ToggleStateOn
			continue
		}
		if deny.Has(perm) {
			perms[name] = templates.ToggleStateOff
			continue
		}
		perms[name] = templates.ToggleStateUnset
	}
	return perms
}

func (s *Server) PatchSettings(w http.ResponseWriter, r *http.Request) {

}
