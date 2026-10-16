package storage

import (
	"os"
	"runtime"
	"testing"

	"github.com/topi314/godrive/server/config"
)

func TestLocalUmaskModes(t *testing.T) {
	s := &localStorage{umask: 0o022}
	if s.dirPerm() != 0o755 {
		t.Fatalf("dir: %04o", s.dirPerm())
	}
	if s.filePerm() != 0o644 {
		t.Fatalf("file: %04o", s.filePerm())
	}
	s.umask = 0
	if s.dirPerm() != 0o777 || s.filePerm() != 0o666 {
		t.Fatalf("umask 0: dir=%04o file=%04o", s.dirPerm(), s.filePerm())
	}
}

func TestLocalUmaskApplied(t *testing.T) {
	dir := t.TempDir()
	st, err := newLocalStorage(config.StorageConfig{
		Local: config.StorageLocalConfig{Path: dir, Umask: 0o022},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Mkdir(t.Context(), "/photos"); err != nil {
		t.Fatal(err)
	}
	abs, err := st.resolve("/photos")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("got %04o want 0755", info.Mode().Perm())
	}
}

func TestS3EndpointSecure(t *testing.T) {
	tests := []struct {
		in, want string
		secure   bool
	}{
		{"localhost:9000", "http://localhost:9000", false},
		{"localhost:9000", "https://localhost:9000", true},
		{"http://minio:9000", "https://minio:9000", true},
		{"https://s3.example", "http://s3.example", false},
	}
	for _, tc := range tests {
		if got := s3Endpoint(tc.in, tc.secure); got != tc.want {
			t.Fatalf("secure=%v %q: got %q want %q", tc.secure, tc.in, got, tc.want)
		}
	}
}
