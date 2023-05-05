package godrive

import (
	"time"
)

type (
	BaseVariables struct {
		Theme string
		Auth  bool
		User  TemplateUser
	}
	IndexVariables struct {
		BaseVariables
		Path      string
		PathParts []string
		Files     []TemplateFile

		Permissions      Permissions
		PermissionRead   Permissions
		PermissionWrite  Permissions
		PermissionCreate Permissions
		PermissionDelete Permissions
		PermissionShare  Permissions
	}

	SettingsVariables struct {
		BaseVariables
		Users       []TemplateUser
		Groups      []string
		Permissions []FilePermissions
	}

	TemplateUser struct {
		ID      string
		Name    string
		Email   string
		Home    string
		IsAdmin bool
		IsUser  bool
		IsGuest bool
	}

	TemplateFile struct {
		IsDir       bool
		Path        string
		Dir         string
		Name        string
		Size        uint64
		Description string
		Date        time.Time
		Owner       string
		Permissions Permissions
	}

	FileRequest struct {
		Size        uint64 `json:"size"`
		Description string `json:"description"`
		Dir         string `json:"dir"`
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

	PermissionsRequest struct {
		Path        string      `json:"path"`
		Permissions Permissions `json:"permissions"`
		ObjectType  ObjectType  `json:"object_type"`
		Object      string      `json:"object"`
	}
)
