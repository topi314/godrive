package godrive

import (
	"encoding/json"
	"errors"
	"golang.org/x/exp/slices"
	"io"
	"log"
	"net/http"
	"path"
	"strconv"
	"strings"
)

func (s *Server) GetFiles(w http.ResponseWriter, r *http.Request) {
	rPath := r.URL.Path

	dl := r.URL.Query().Get("dl")
	if dl != "" && dl != "0" {
		s.serveFiles(w, r, rPath)
		return
	}

	files, err := s.db.FindFiles(r.Context(), rPath)
	if err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	if len(files) == 1 && path.Join(files[0].Dir, files[0].Name) == rPath {
		w.Header().Set("Content-Type", files[0].ContentType)
		w.Header().Set("Content-Length", strconv.FormatUint(files[0].Size, 10))
		if err = s.writeFile(r.Context(), w, rPath); err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		return
	}

	var templateFiles []TemplateFile
	for _, file := range files {
		if file.Private {
			continue
		}
		updatedAt := file.UpdatedAt
		if updatedAt.IsZero() {
			updatedAt = file.CreatedAt
		}

		relativePath := strings.TrimPrefix(file.Dir, rPath)
		if relativePath != "" {
			if strings.Count(relativePath, "") > 0 {
				parts := strings.Split(relativePath, "/")
				if len(parts) > 1 {
					relativePath = parts[1]
				} else {
					relativePath = parts[0]
				}
			}

			index := slices.IndexFunc(templateFiles, func(f TemplateFile) bool {
				return f.Name == relativePath
			})
			if index > -1 {
				templateFiles[index].Size += file.Size
				if templateFiles[index].Date.Before(updatedAt) {
					templateFiles[index].Date = updatedAt
				}
				continue
			}

			templateFiles = append(templateFiles, TemplateFile{
				IsDir:       true,
				Name:        relativePath,
				Dir:         rPath,
				Size:        file.Size,
				Description: "",
				Date:        updatedAt,
			})
			continue
		}

		templateFiles = append(templateFiles, TemplateFile{
			IsDir:       false,
			Name:        file.Name,
			Dir:         file.Dir,
			Size:        file.Size,
			Description: file.Description,
			Date:        updatedAt,
		})
	}

	vars := TemplateVariables{
		Path:      rPath,
		PathParts: strings.FieldsFunc(rPath, func(r rune) bool { return r == '/' }),
		Files:     templateFiles,
		Theme:     "dark",
	}
	if err = s.tmpl(w, "index.gohtml", vars); err != nil {
		log.Println("failed to execute template:", err)
	}
}

func (s *Server) PostFiles(w http.ResponseWriter, r *http.Request) {
	if err := s.parseMultipart(r, func(file ParsedFile, reader io.Reader) error {
		if _, err := s.db.CreateFile(r.Context(), file.Dir, file.Name, file.Size, file.ContentType, file.Description, file.Private); err != nil {
			return err
		}

		return s.storage.PutObject(r.Context(), path.Join(file.Dir, file.Name), file.Size, reader, file.ContentType)
	}); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	s.ok(w, r, nil)
}

func (s *Server) PatchFiles(w http.ResponseWriter, r *http.Request) {
	if err := s.parseMultipart(r, func(file ParsedFile, reader io.Reader) error {
		if err := s.db.UpdateFile(r.Context(), file.Dir, file.Name, file.Size, file.ContentType, file.Description, file.Private); err != nil {
			return err
		}

		return s.storage.PutObject(r.Context(), path.Join(file.Dir, file.Name), file.Size, reader, file.ContentType)
	}); err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	s.ok(w, r, nil)
}

func (s *Server) DeleteFiles(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var files []string
	if err := json.NewDecoder(r.Body).Decode(&files); err != nil {
		s.error(w, r, err, http.StatusBadRequest)
		return
	}

	var finalErr error
	for _, file := range files {
		if err := s.db.DeleteFile(r.Context(), r.URL.Path, file); err != nil {
			finalErr = errors.Join(finalErr, err)
			continue
		}
		if err := s.storage.DeleteObject(r.Context(), path.Join(r.URL.Path, file)); err != nil {
			finalErr = errors.Join(finalErr, err)
			continue
		}
	}

	if finalErr != nil {
		s.error(w, r, finalErr, http.StatusInternalServerError)
		return
	}

	s.json(w, r, nil, http.StatusNoContent)
}

type ParsedFile struct {
	Dir         string
	Name        string
	Size        uint64
	ContentType string
	Description string
	Private     bool
}

func (s *Server) parseMultipart(r *http.Request, fileFunc func(file ParsedFile, reader io.Reader) error) error {
	mr, err := r.MultipartReader()
	if err != nil {
		return err
	}

	part, err := mr.NextPart()
	if err != nil {
		return err
	}
	defer part.Close()

	if part.FormName() != "json" {
		return errors.New("json field not found")
	}

	var files []FileRequest
	if err = json.NewDecoder(part).Decode(&files); err != nil {
		return err
	}

	for _, file := range files {
		part, err = mr.NextPart()
		if err == io.EOF {
			return errors.New("not enough files")
		}
		if err != nil {
			return err
		}

		contentType := part.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "application/octet-stream"
		}

		parsedFile := ParsedFile{
			Dir:         r.URL.Path,
			Name:        part.FileName(),
			Size:        file.Size,
			ContentType: contentType,
			Description: file.Description,
			Private:     file.Private,
		}

		if err = fileFunc(parsedFile, part); err != nil {
			return err
		}

	}
	return nil
}
