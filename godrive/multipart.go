package godrive

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
)

type MultipartFilePart struct {
	Dir         string
	Name        string
	Description string
	Overwrite   bool
	Size        uint64
	Content     func() (*FilePart, error)
}

func (p MultipartFilePart) Path() string {
	return path.Join(p.Dir, p.Name)
}

type FilePart struct {
	Reader      io.Reader
	ContentType string
}

func (s *Server) parseMultiparts(r *http.Request) ([]MultipartFilePart, error) {
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

func (s *Server) parseMultipart(r *http.Request) (*MultipartFilePart, error) {
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
