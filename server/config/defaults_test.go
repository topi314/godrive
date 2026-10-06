package config

import (
	"testing"
	"time"
)

func TestExpandHome(t *testing.T) {
	if got := ExpandHome("/home/{user}", "alice", "", ""); got != "/home/alice" {
		t.Fatalf("got %q", got)
	}
	if got := ExpandHome("/home/{user}", "../etc", "ok@x", "id"); got != "/home/ok@x" {
		t.Fatalf("unsafe user fell back to email: %q", got)
	}
}

func TestApplyDefaultsSyncInterval(t *testing.T) {
	local := &Config{Storage: StorageConfig{Type: StorageTypeLocal}}
	applyDefaults(local)
	if local.Storage.SyncInterval == nil || local.Storage.SyncInterval.Duration != DefaultSyncInterval {
		t.Fatalf("local: got %v, want %v", local.Storage.SyncInterval, DefaultSyncInterval)
	}

	s3 := &Config{Storage: StorageConfig{Type: StorageTypeS3}}
	applyDefaults(s3)
	if s3.Storage.SyncInterval == nil || s3.Storage.SyncInterval.Duration != DefaultS3SyncInterval {
		t.Fatalf("s3: got %v, want %v", s3.Storage.SyncInterval, DefaultS3SyncInterval)
	}

	explicit := &Config{Storage: StorageConfig{
		Type:         StorageTypeS3,
		SyncInterval: &Duration{Duration: 5 * time.Minute},
	}}
	applyDefaults(explicit)
	if explicit.Storage.SyncInterval.Duration != 5*time.Minute {
		t.Fatalf("explicit: got %v, want 5m", explicit.Storage.SyncInterval.Duration)
	}
}
