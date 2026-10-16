package config

import "testing"

func TestByteSizeUnmarshal(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"16MB", 16_000_000},
		{"50GB", 50_000_000_000},
		{"1024", 1024},
		{"1MiB", 1024 * 1024},
	}
	for _, tc := range cases {
		var b ByteSize
		if err := b.UnmarshalText([]byte(tc.in)); err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if b.Bytes != tc.want {
			t.Fatalf("%s: got %d want %d", tc.in, b.Bytes, tc.want)
		}
	}
}

func TestUploadDefaults(t *testing.T) {
	cfg := Config{}
	applyDefaults(&cfg)
	if cfg.Upload.MaxSize.Bytes != DefaultUploadMax {
		t.Fatalf("max: %d", cfg.Upload.MaxSize.Bytes)
	}
	if cfg.Upload.ChunkSize.Bytes != DefaultUploadChunk {
		t.Fatalf("chunk: %d", cfg.Upload.ChunkSize.Bytes)
	}
	if cfg.Upload.SessionTTL.Duration != DefaultUploadTTL {
		t.Fatalf("ttl: %v", cfg.Upload.SessionTTL.Duration)
	}
	if cfg.Upload.MaxParallel != DefaultUploadParallel {
		t.Fatalf("parallel: %d", cfg.Upload.MaxParallel)
	}
}
