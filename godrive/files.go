package godrive

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/topi314/godrive/templates"
	"golang.org/x/exp/slices"
)

func (s *Server) GetFiles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var (
		download    bool
		filesFilter []string
	)
	if dl := query.Get("dl"); dl == "1" || strings.ToLower(dl) == "true" {
		download = true
		if queryFiles := query.Get("files"); queryFiles != "" {
			filesFilter = strings.Split(queryFiles, ",")
		}
	}

	files, err := s.db.FindFiles(r.Context(), r.URL.Path)
	if err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	}

	if download && len(files) == 0 {
		s.notFound(w, r)
		return
	}
	if r.URL.Path != "/" && len(files) == 0 {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	userInfo := GetUserInfo(r)
	if len(files) == 1 && files[0].Path == r.URL.Path {
		start, end, err := parseRange(r.Header.Get("Range"))
		if err != nil {
			s.error(w, r, err, http.StatusRequestedRangeNotSatisfiable)
			return
		}
		file := files[0]
		if download {
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
				"filename": path.Base(file.Path),
			}))
		}
		w.Header().Set("Content-Type", file.ContentType)
		w.Header().Set("Content-Length", strconv.FormatUint(file.Size, 10))
		w.Header().Set("Accept-Ranges", "bytes")
		if start != nil || end != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, file.Size))
			w.WriteHeader(http.StatusPartialContent)
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err = s.writeFile(r.Context(), w, file.Path, start, end); err != nil {
			slog.ErrorContext(r.Context(), "Failed to write file", slog.Any("err", err))
		}
		return
	}

	if download {
		zipName := path.Base(r.URL.Path)
		if zipName == "/" || zipName == "." {
			zipName = "godrive"
		}

		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{
			"filename": zipName + ".zip",
		}))

		zw := zip.NewWriter(w)
		defer zw.Close()

		rPath := r.URL.Path
		if !strings.HasSuffix(rPath, "/") {
			rPath += "/"
		}

		var addedFiles int
		for _, file := range files {
			if len(filesFilter) > 0 && !slices.Contains(filesFilter, strings.TrimPrefix(file.Path, rPath)) {
				continue
			}
			fw, err := zw.CreateHeader(&zip.FileHeader{
				Name:               strings.TrimPrefix(file.Path, "/"),
				UncompressedSize64: file.Size,
				Modified:           file.UpdatedAt,
				Comment:            file.Description,
				Method:             zip.Deflate,
			})
			addedFiles++
			if err != nil {
				s.error(w, r, err, http.StatusInternalServerError)
				return
			}
			if err = s.writeFile(r.Context(), fw, file.Path, nil, nil); err != nil {
				s.error(w, r, err, http.StatusInternalServerError)
				return
			}
		}
		if addedFiles == 0 {
			s.notFound(w, r)
			return
		}
		if err = zw.SetComment("Generated by godrive"); err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		return
	}

	var templateFiles []templates.File
	for _, file := range files {
		owner := "Unknown"
		if file.Username != nil {
			owner = *file.Username
		}
		isOwner := file.UserID == userInfo.Subject || s.isAdmin(userInfo)
		date := file.CreatedAt
		if file.UpdatedAt.After(date) {
			date = file.UpdatedAt
		}

		if dir := strings.TrimPrefix(path.Dir(file.Path), r.URL.Path); dir != "" {
			baseDir := strings.TrimPrefix(dir, "/")
			if strings.Count(baseDir, "/") > 0 {
				baseDir = strings.SplitN(baseDir, "/", 2)[0]
			}
			index := slices.IndexFunc(templateFiles, func(file templates.File) bool {
				return file.Name == baseDir
			})
			if index == -1 {
				templateFiles = append(templateFiles, templates.File{
					IsDir:   true,
					Path:    path.Join(r.URL.Path, baseDir),
					Dir:     r.URL.Path,
					Name:    baseDir,
					Size:    file.Size,
					Date:    date,
					Owner:   owner,
					IsOwner: isOwner,
				})
				continue
			}
			templateFiles[index].Size += file.Size
			if templateFiles[index].Date.Before(date) {
				templateFiles[index].Date = date
			}
			if !strings.Contains(templateFiles[index].Owner, owner) {
				templateFiles[index].Owner += ", " + owner
			}
			if !templateFiles[index].IsOwner && isOwner {
				templateFiles[index].IsOwner = true
			}
			continue
		}

		templateFiles = append(templateFiles, s.newTemplateFile(userInfo, file))
	}

	action := strings.ToLower(query.Get("action"))
	if action == "files" {
		if err = templates.FileList(s.cfg.Auth != nil, templateFiles).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "error executing template", slog.Any("err", err))
		}
		return
	}

	vars := templates.IndexVars{
		Theme:     "dark",
		Auth:      s.cfg.Auth != nil,
		User:      s.newTemplateUser(userInfo),
		Path:      r.URL.Path,
		PathParts: strings.FieldsFunc(r.URL.Path, func(r rune) bool { return r == '/' }),
		Files:     templateFiles,
	}

	if action == "main" {
		w.Header().Set("HX-Replace-Url", r.URL.Path)
		if err = templates.Main(vars).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "error executing template", slog.Any("err", err))
		}
		return
	}

	if err = templates.Index(vars).Render(r.Context(), w); err != nil {
		slog.ErrorContext(r.Context(), "error executing template", slog.Any("err", err))
	}
}

func (s *Server) PostFile(w http.ResponseWriter, r *http.Request) {
	action := strings.ToLower(r.URL.Query().Get("action"))
	if action == "upload" {
		if err := templates.UploadFile(r.URL.Path).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "error executing template", slog.Any("err", err))
		}
		return
	}
	files, err := s.parseMultiparts(r)
	if err != nil {
		s.error(w, r, err, http.StatusBadRequest)
		return
	}

	userInfo := GetUserInfo(r)
	for _, file := range files {
		filePart, err := file.Content()
		if err != nil {
			s.error(w, r, err, http.StatusBadRequest)
			return
		}

		var tx *sqlx.Tx
		if file.Overwrite {
			_, tx, err = s.db.CreateOrUpdateFile(r.Context(), file.Path(), file.Size, filePart.ContentType, file.Description, userInfo.Subject)
		} else {
			_, tx, err = s.db.CreateFile(r.Context(), file.Path(), file.Size, filePart.ContentType, file.Description, userInfo.Subject)
		}

		if err != nil {
			if errors.Is(err, ErrFileAlreadyExists) {
				s.error(w, r, err, http.StatusBadRequest)
				return
			}
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		if err = s.storage.PutObject(r.Context(), file.Path(), file.Size, filePart.Reader, filePart.ContentType); err != nil {
			if txErr := tx.Rollback(); txErr != nil {
				slog.ErrorContext(r.Context(), "error rolling back transaction", slog.Any("err", txErr))
			}
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		if err = tx.Commit(); err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
	}

	if r.Header.Get("Accept") == "text/html" {
		http.Redirect(w, r, r.URL.Path+"?action=main", http.StatusSeeOther)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) PatchFile(w http.ResponseWriter, r *http.Request) {
	files, err := s.db.FindFiles(r.Context(), r.URL.Path)
	if err != nil {
		s.error(w, r, err, http.StatusInternalServerError)
		return
	} else if len(files) == 0 {
		s.error(w, r, err, http.StatusNotFound)
		return
	}

	query := r.URL.Query()
	action := strings.ToLower(query.Get("action"))
	userInfo := GetUserInfo(r)
	// update specific file
	if len(files) == 1 && files[0].Path == r.URL.Path {
		dbFile := files[0]
		if !s.hasFileAccess(userInfo, dbFile) {
			s.error(w, r, ErrUnauthorized, http.StatusUnauthorized)
			return
		}

		if action == "edit" {
			if err = templates.EditFile(s.newTemplateFile(userInfo, dbFile)).Render(r.Context(), w); err != nil {
				slog.ErrorContext(r.Context(), "error executing template", slog.Any("err", err))
			}
			return
		}

		file, err := s.parseMultipart(r)
		if err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		filePart, err := file.Content()
		if err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		tx, err := s.db.UpdateFile(r.Context(), r.URL.Path, file.Path(), file.Size, filePart.ContentType, file.Description)
		if err != nil {
			if errors.Is(err, ErrFileNotFound) {
				s.error(w, r, err, http.StatusNotFound)
				return
			}
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		if file.Size > 0 {
			if err = s.storage.PutObject(r.Context(), file.Path(), file.Size, filePart.Reader, filePart.ContentType); err != nil {
				if txErr := tx.Rollback(); txErr != nil {
					slog.Error("error rolling back transaction", slog.Any("err", txErr))
				}
				s.error(w, r, err, http.StatusInternalServerError)
				return
			}
			if r.URL.Path != file.Path() {
				if err = s.storage.DeleteObject(r.Context(), r.URL.Path); err != nil {
					if txErr := tx.Rollback(); txErr != nil {
						slog.Error("error rolling back transaction", slog.Any("err", txErr))
					}
					s.error(w, r, err, http.StatusInternalServerError)
					return
				}
			}
		} else if r.URL.Path != file.Path() {
			if err = s.storage.MoveObject(r.Context(), r.URL.Path, file.Path()); err != nil {
				if txErr := tx.Rollback(); txErr != nil {
					slog.Error("error rolling back transaction", slog.Any("err", txErr))
				}
				s.error(w, r, err, http.StatusInternalServerError)
				return
			}
		}

		if err = tx.Commit(); err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		if r.Header.Get("Accept") == "text/html" {
			http.Redirect(w, r, path.Dir(r.URL.Path)+"?action=main", http.StatusSeeOther)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		return
	}

	var filesFilter []string
	if queryFiles := query.Get("files"); queryFiles != "" {
		filesFilter = strings.Split(queryFiles, ",")
	}
	// update multiple files
	if action == "move" {
		var queryFiles string
		if len(filesFilter) > 0 {
			queryFiles = "?files=" + url.QueryEscape(strings.Join(filesFilter, ","))
		}
		if err = templates.MoveFile(r.URL.Path, queryFiles).Render(r.Context(), w); err != nil {
			slog.ErrorContext(r.Context(), "error executing template", slog.Any("err", err))
		}
		return
	}

	destination := r.Header.Get("Destination")
	if destination == "" {
		s.error(w, r, errors.New("missing destination header"), http.StatusBadRequest)
		return
	}
	rPath := r.URL.Path
	if !strings.HasSuffix(rPath, "/") {
		rPath += "/"
	}
	var errs error
	for _, file := range files {
		if len(filesFilter) > 0 && !slices.Contains(filesFilter, strings.SplitN(strings.TrimPrefix(file.Path, rPath), "/", 2)[0]) {
			continue
		}
		if !s.hasFileAccess(userInfo, file) {
			continue
		}

		newPath := path.Join(destination, strings.TrimPrefix(file.Path, rPath))
		tx, err := s.db.UpdateFile(r.Context(), file.Path, newPath, 0, "", file.Description)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}

		if err = s.storage.MoveObject(r.Context(), file.Path, newPath); err != nil {
			if txErr := tx.Rollback(); txErr != nil {
				slog.Error("error rolling back transaction", slog.Any("err", txErr))
			}
			errs = errors.Join(errs, err)
			continue
		}
	}

	if errs != nil {
		s.error(w, r, errs, http.StatusInternalServerError)
		return
	}

	if r.Header.Get("Accept") == "text/html" {
		http.Redirect(w, r, r.URL.Path+"?action=main", http.StatusSeeOther)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) DeleteFiles(w http.ResponseWriter, r *http.Request) {
	var filesFilter []string
	if queryFiles := r.URL.Query().Get("files"); queryFiles != "" {
		filesFilter = strings.Split(queryFiles, ",")
	}

	files, err := s.db.FindFiles(r.Context(), r.URL.Path)
	if err != nil {
		return
	}

	if len(files) == 0 {
		s.error(w, r, errors.New("file not found"), http.StatusNotFound)
		return
	}

	userInfo := GetUserInfo(r)
	// delete specific file
	if len(files) == 1 && files[0].Path == r.URL.Path {
		if !s.hasFileAccess(userInfo, files[0]) {
			s.error(w, r, fmt.Errorf("unauthorized to delete file: %s", files[0].Path), http.StatusUnauthorized)
			return
		}
		tx, err := s.db.DeleteFile(r.Context(), files[0].Path)
		if err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		if err = s.storage.DeleteObject(r.Context(), files[0].Path); err != nil {
			if txErr := tx.Rollback(); txErr != nil {
				slog.Error("error rolling back transaction", slog.Any("err", txErr))
			}
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}
		if err = tx.Commit(); err != nil {
			s.error(w, r, err, http.StatusInternalServerError)
			return
		}

		if r.Header.Get("Accept") == "text/html" {
			http.Redirect(w, r, path.Dir(r.URL.Path)+"?action=main", http.StatusSeeOther)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	rPath := r.URL.Path
	if !strings.HasSuffix(rPath, "/") {
		rPath += "/"
	}
	var (
		errs  error
		warns []string
	)
	for _, file := range files {
		if len(filesFilter) > 0 && !slices.Contains(filesFilter, strings.SplitN(strings.TrimPrefix(file.Path, rPath), "/", 2)[0]) {
			continue
		}
		if !s.hasFileAccess(userInfo, file) {
			warns = append(warns, fmt.Sprintf("unauthorized to delete file: %s", file.Path))
			continue
		}
		tx, err := s.db.DeleteFile(r.Context(), file.Path)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if err = s.storage.DeleteObject(r.Context(), file.Path); err != nil {
			if txErr := tx.Rollback(); txErr != nil {
				slog.Error("error rolling back transaction", slog.Any("err", txErr))
			}
			errs = errors.Join(errs, err)
			continue
		}
		if err = tx.Commit(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if errs != nil {
		s.error(w, r, errs, http.StatusInternalServerError)
		return
	}
	if len(warns) > 0 {
		s.warn(w, r, strings.Join(warns, ", "), http.StatusMultiStatus)
		return
	}

	if r.Header.Get("Accept") == "text/html" {
		http.Redirect(w, r, r.URL.Path+"?action=main", http.StatusSeeOther)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) writeFile(ctx context.Context, w io.Writer, fullPath string, start *int64, end *int64) error {
	obj, err := s.storage.GetObject(ctx, fullPath, start, end)
	if err != nil {
		return err
	}
	defer obj.Close()
	if _, err = io.Copy(w, obj); err != nil {
		return err
	}
	return nil
}

func parseRange(rangeHeader string) (*int64, *int64, error) {
	if rangeHeader == "" {
		return nil, nil, nil
	}

	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return nil, nil, errors.New("invalid range header, must start with 'bytes='")
	}

	var (
		start int64
		end   int64
	)
	if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err == nil {
		return &start, &end, nil
	}
	if _, err := fmt.Sscanf(rangeHeader, "bytes=-%d", &end); err == nil {
		return nil, &end, nil
	}
	if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-", &start); err == nil {
		return &start, nil, nil
	}

	return nil, nil, fmt.Errorf("invalid range header: %s", rangeHeader)
}
