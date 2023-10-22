package godrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/topi314/godrive/godrive/auth"
	"github.com/topi314/godrive/godrive/database"
	"github.com/topi314/godrive/templates"
)

type (
	FileCreateRequest struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Overwrite   bool        `json:"overwrite"`
		Size        int64       `json:"size"`
		Permissions Permissions `json:"permissions"`
	}

	FileUpdateRequest struct {
		Dir         string      `json:"dir"`
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Size        int64       `json:"size"`
		Permissions Permissions `json:"permissions"`
	}

	Permissions []Permission

	Permission struct {
		ObjectType  int                              `json:"object_type"`
		Object      string                           `json:"object"`
		Permissions map[string]templates.ToggleState `json:"permissions"`
	}

	MultipartFilePart struct {
		Dir         string
		Name        string
		Description string
		Overwrite   bool
		Size        int64
		Permissions Permissions
		Content     func() (*FilePart, error)
	}
)

func (p Permission) calculate() (auth.Permissions, auth.Permissions) {
	var (
		allow auth.Permissions
		deny  auth.Permissions
	)

	for perm, state := range p.Permissions {
		for permBit, action := range auth.AllPermissions {
			if perm != action {
				continue
			}

			switch state {
			case templates.ToggleStateOn:
				allow = allow.Add(permBit)
			case templates.ToggleStateOff:
				deny = deny.Add(permBit)
			}
		}
	}

	return allow, deny
}

func (p Permissions) ToDatabase(path string) []database.Permissions {
	perms := make([]database.Permissions, len(p))
	for i, perm := range p {
		allow, deny := perm.calculate()
		perms[i] = database.Permissions{
			Path:       path,
			Allow:      int64(allow),
			Deny:       int64(deny),
			ObjectType: perm.ObjectType,
			Object:     perm.Object,
		}
	}
	return perms
}

func (p MultipartFilePart) Path() string {
	return path.Join(p.Dir, p.Name)
}

type FilePart struct {
	Reader      io.Reader
	ContentType string
}

func ParseMultiparts(r *http.Request) ([]MultipartFilePart, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}

	part, err := mr.NextPart()
	if err != nil {
		return nil, err
	}

	if part.FormName() != "json" {
		return nil, fmt.Errorf("expected json field, got %s", part.FormName())
	}

	var files []FileCreateRequest
	if err = json.NewDecoder(part).Decode(&files); err != nil {
		return nil, err
	}

	var parsedFiles []MultipartFilePart
	for i, file := range files {
		i := i
		parsedFiles = append(parsedFiles, MultipartFilePart{
			Dir:         r.URL.Path,
			Name:        file.Name,
			Description: file.Description,
			Overwrite:   file.Overwrite,
			Size:        file.Size,
			Permissions: file.Permissions,
			Content: func() (*FilePart, error) {
				part, err = mr.NextPart()
				if err != nil {
					return nil, err
				}
				if part.FormName() != fmt.Sprintf("file-%d", i) {
					return nil, fmt.Errorf("expected file-%d field, got %s", i, part.FormName())
				}

				contentType := part.Header.Get("Content-Type")
				if contentType == "" {
					contentType = "application/octet-stream"
				}

				return &FilePart{
					Reader:      part,
					ContentType: contentType,
				}, nil
			},
		})
	}

	return parsedFiles, nil
}

func ParseMultipart(r *http.Request) (*MultipartFilePart, error) {
	mr, err := r.MultipartReader()
	if errors.Is(err, io.EOF) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	part, err := mr.NextPart()
	if err != nil {
		return nil, err
	}

	if part.FormName() != "json" {
		return nil, fmt.Errorf("expected json field, got %s", part.FormName())
	}

	var file FileUpdateRequest
	if err = json.NewDecoder(part).Decode(&file); err != nil {
		return nil, err
	}

	dir := file.Dir
	if dir == "" {
		dir = path.Dir(r.URL.Path)
	}

	return &MultipartFilePart{
		Dir:         dir,
		Name:        file.Name,
		Description: file.Description,
		Size:        file.Size,
		Permissions: file.Permissions,
		Content: func() (*FilePart, error) {
			part, err = mr.NextPart()
			if errors.Is(err, io.EOF) {
				return &FilePart{}, nil
			} else if err != nil {
				return nil, err
			}

			if part.FormName() != "file" {
				return nil, fmt.Errorf("expected file field, got %s", part.FormName())
			}

			contentType := part.Header.Get("Content-Type")
			if contentType == "" {
				contentType = "application/octet-stream"
			}

			return &FilePart{
				Reader:      part,
				ContentType: contentType,
			}, nil
		},
	}, nil
}
