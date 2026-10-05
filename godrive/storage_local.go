package godrive

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
)

type localStorage struct {
	root string
}

func newLocalStorage(cfg StorageConfig) (*localStorage, error) {
	root, err := filepath.Abs(cfg.Path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &localStorage{root: root}, nil
}

func (s *localStorage) resolve(p string) (string, error) {
	p = strings.TrimPrefix(NormalizePath(p), "/")
	full := filepath.Join(s.root, filepath.FromSlash(p))
	abs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(abs, s.root) {
		return "", fmt.Errorf("path escapes storage root")
	}
	return abs, nil
}

func (s *localStorage) toLogical(abs string) string {
	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return "/"
	}
	return NormalizePath("/" + filepath.ToSlash(rel))
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
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	tmp := abs + ".tmp"
	f, err := os.Create(tmp)
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

func (s *localStorage) DeleteObject(ctx context.Context, path string) error {
	abs, err := s.resolve(path)
	if err != nil {
		return err
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

func (s *localStorage) Stat(ctx context.Context, path string) (ObjectInfo, error) {
	abs, err := s.resolve(path)
	if err != nil {
		return ObjectInfo{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return ObjectInfo{}, err
	}
	ct := mime.TypeByExtension(filepath.Ext(abs))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return ObjectInfo{
		Path:         NormalizePath(path),
		Size:         st.Size(),
		ContentType:  ct,
		LastModified: st.ModTime(),
	}, nil
}

func (s *localStorage) List(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	prefix = NormalizePath(prefix)
	var out []ObjectInfo
	err := filepath.WalkDir(s.root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
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

func (s *localStorage) Watch(ctx context.Context, onChange func(StorageEvent)) error {
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
						onChange(StorageEvent{Type: StorageEventDelete, Path: path})
						return
					}
					info, err := s.Stat(ctx, path)
					if err != nil {
						return
					}
					onChange(StorageEvent{Type: StorageEventUpsert, Path: path, Info: info})
				})
			}
		}
	}()
	return nil
}

func (s *localStorage) Close() error { return nil }
