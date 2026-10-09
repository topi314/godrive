package storage

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/topi314/godrive/server/acl"
	"github.com/topi314/godrive/server/config"
)

type localStorage struct {
	root  string
	umask int
}

func newLocalStorage(cfg config.StorageConfig) (*localStorage, error) {
	root, err := filepath.Abs(cfg.Local.Path)
	if err != nil {
		return nil, err
	}
	s := &localStorage{root: root, umask: cfg.Local.Umask & 0o777}
	if err := s.mkdirAll(root); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *localStorage) dirPerm() os.FileMode {
	return os.FileMode(0o777 &^ s.umask)
}

func (s *localStorage) filePerm() os.FileMode {
	return os.FileMode(0o666 &^ s.umask)
}

// mkdirAll creates path and any missing parents, then forces the configured dir mode
// (so the process umask does not override [storage.local].umask).
func (s *localStorage) mkdirAll(path string) error {
	if err := os.MkdirAll(path, s.dirPerm()); err != nil {
		return err
	}
	return os.Chmod(path, s.dirPerm())
}

func (s *localStorage) openFile(path string, flag int) (*os.File, error) {
	f, err := os.OpenFile(path, flag, s.filePerm())
	if err != nil {
		return nil, err
	}
	if flag&os.O_CREATE != 0 {
		if err := f.Chmod(s.filePerm()); err != nil {
			_ = f.Close()
			return nil, err
		}
	}
	return f, nil
}

func (s *localStorage) resolve(p string) (string, error) {
	p = strings.TrimPrefix(acl.NormalizePath(p), "/")
	full := filepath.Join(s.root, filepath.FromSlash(p))
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	// Rel rejects paths outside root (also avoids /data vs /data2 prefix bugs).
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes storage root")
	}
	return abs, nil
}

func (s *localStorage) toLogical(abs string) string {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return "/"
	}
	return acl.NormalizePath("/" + filepath.ToSlash(rel))
}

func (s *localStorage) GetObject(ctx context.Context, path string, start, end int64) (io.ReadCloser, ObjectInfo, error) {
	info, err := s.Stat(ctx, path)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	abs, err := s.resolve(path)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	if start > 0 {
		if _, err := f.Seek(start, io.SeekStart); err != nil {
			_ = f.Close()
			return nil, ObjectInfo{}, err
		}
	}
	if end >= start && end > 0 {
		return struct {
			io.Reader
			io.Closer
		}{Reader: io.LimitReader(f, end-start+1), Closer: f}, info, nil
	}
	return f, info, nil
}

func (s *localStorage) PutObject(ctx context.Context, path string, size int64, r io.Reader, contentType string) error {
	abs, err := s.resolve(path)
	if err != nil {
		return err
	}
	if err := s.mkdirAll(filepath.Dir(abs)); err != nil {
		return err
	}
	tmp := abs + ".tmp"
	f, err := s.openFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, abs)
}

func (s *localStorage) Mkdir(ctx context.Context, path string) error {
	abs, err := s.resolve(path)
	if err != nil {
		return err
	}
	return s.mkdirAll(abs)
}

func (s *localStorage) DeleteObject(ctx context.Context, path string) error {
	abs, err := s.resolve(path)
	if err != nil {
		return err
	}
	st, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if st.IsDir() {
		return os.RemoveAll(abs)
	}
	return os.Remove(abs)
}

func (s *localStorage) CopyObject(ctx context.Context, from, to string) error {
	src, info, err := s.GetObject(ctx, from, 0, -1)
	if err != nil {
		return err
	}
	defer src.Close()
	return s.PutObject(ctx, to, info.Size, src, info.ContentType)
}

func (s *localStorage) RenameObject(ctx context.Context, from, to string) error {
	src, err := s.resolve(from)
	if err != nil {
		return err
	}
	dst, err := s.resolve(to)
	if err != nil {
		return err
	}
	if err := s.mkdirAll(filepath.Dir(dst)); err != nil {
		return err
	}
	return os.Rename(src, dst)
}

func (s *localStorage) Stat(ctx context.Context, path string) (ObjectInfo, error) {
	abs, err := s.resolve(path)
	if err != nil {
		return ObjectInfo{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return ObjectInfo{}, err
	}
	if st.IsDir() {
		return ObjectInfo{
			Path:         acl.NormalizePath(path),
			Size:         0,
			ContentType:  ContentTypeDirectory,
			LastModified: st.ModTime(),
		}, nil
	}
	ct := mime.TypeByExtension(filepath.Ext(abs))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ObjectInfo{
		Path:         acl.NormalizePath(path),
		Size:         st.Size(),
		ContentType:  ct,
		LastModified: st.ModTime(),
	}, nil
}

func (s *localStorage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	prefix = acl.NormalizePath(prefix)
	var out []ObjectInfo
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == s.root {
				return nil
			}
			logical := s.toLogical(path)
			if prefix != "/" && logical != prefix && !strings.HasPrefix(logical, prefix+"/") {
				return nil
			}
			info, err := s.Stat(ctx, logical)
			if err != nil {
				return nil
			}
			out = append(out, info)
			return nil
		}
		logical := s.toLogical(path)
		if prefix != "/" && logical != prefix && !strings.HasPrefix(logical, prefix+"/") {
			return nil
		}
		info, err := s.Stat(ctx, logical)
		if err != nil {
			return nil
		}
		out = append(out, info)
		return nil
	})
	return out, err
}

func (s *localStorage) Watch(ctx context.Context, onChange func(Event)) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	var addDir func(string)
	addDir = func(dir string) {
		_ = watcher.Add(dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				addDir(filepath.Join(dir, e.Name()))
			}
		}
	}
	addDir(s.root)

	debounce := map[string]*time.Timer{}
	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				_ = err
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Has(fsnotify.Create) {
					if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
						addDir(ev.Name)
						logical := s.toLogical(ev.Name)
						info, err := s.Stat(ctx, logical)
						if err == nil {
							onChange(Event{Type: EventUpsert, Path: logical, Info: info})
						}
						continue
					}
				}
				logical := s.toLogical(ev.Name)
				if strings.HasSuffix(logical, ".tmp") {
					continue
				}
				if t, ok := debounce[logical]; ok {
					t.Stop()
				}
				path := logical
				debounce[path] = time.AfterFunc(200*time.Millisecond, func() {
					delete(debounce, path)
					if ev.Has(fsnotify.Remove) || ev.Has(fsnotify.Rename) {
						onChange(Event{Type: EventDelete, Path: path})
						return
					}
					info, err := s.Stat(ctx, path)
					if err != nil {
						return
					}
					onChange(Event{Type: EventUpsert, Path: path, Info: info})
				})
			}
		}
	}()
	return nil
}

func (s *localStorage) Close() error { return nil }

func (s *localStorage) uploadAbs(tempKey string) (string, error) {
	if tempKey == "" || strings.Contains(tempKey, "..") || strings.ContainsAny(tempKey, "/\\") {
		return "", fmt.Errorf("invalid upload key")
	}
	dir := filepath.Join(s.root, ".uploads")
	if err := s.mkdirAll(dir); err != nil {
		return "", err
	}
	return filepath.Join(dir, tempKey), nil
}

func (s *localStorage) CreateUpload(ctx context.Context, tempKey string, size int64) (string, error) {
	abs, err := s.uploadAbs(tempKey)
	if err != nil {
		return "", err
	}
	f, err := s.openFile(abs, os.O_CREATE|os.O_TRUNC|os.O_WRONLY)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if size > 0 {
		if err := f.Truncate(size); err != nil {
			_ = os.Remove(abs)
			return "", err
		}
	}
	return "", nil
}

func (s *localStorage) WriteUpload(ctx context.Context, tempKey string, uploadOffset int64, r io.Reader, n int64, uploadID string, partNumber int32) (string, error) {
	abs, err := s.uploadAbs(tempKey)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(abs, os.O_RDWR, s.filePerm())
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Seek(uploadOffset, io.SeekStart); err != nil {
		return "", err
	}
	written, err := io.Copy(f, io.LimitReader(r, n))
	if err != nil {
		return "", err
	}
	if written != n {
		return "", fmt.Errorf("short write: %d/%d", written, n)
	}
	return "", nil
}

func (s *localStorage) CommitUpload(ctx context.Context, tempKey, destPath, contentType, uploadID string, parts []CompletedPart) error {
	abs, err := s.uploadAbs(tempKey)
	if err != nil {
		return err
	}
	dest, err := s.resolve(destPath)
	if err != nil {
		return err
	}
	if err := s.mkdirAll(filepath.Dir(dest)); err != nil {
		return err
	}
	if err := os.Rename(abs, dest); err != nil {
		// Cross-device fallback.
		in, err := os.Open(abs)
		if err != nil {
			return err
		}
		defer in.Close()
		st, _ := in.Stat()
		size := int64(0)
		if st != nil {
			size = st.Size()
		}
		if err := s.PutObject(ctx, destPath, size, in, contentType); err != nil {
			return err
		}
		_ = os.Remove(abs)
	}
	return nil
}

func (s *localStorage) AbortUpload(ctx context.Context, tempKey, uploadID string) error {
	abs, err := s.uploadAbs(tempKey)
	if err != nil {
		return err
	}
	_ = os.Remove(abs)
	return nil
}

