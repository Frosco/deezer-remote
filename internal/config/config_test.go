package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadFromPath_ReturnsArl(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("arl = \"abc123\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatalf("loadFromPath: %v", err)
	}
	if cfg.ARL != "abc123" {
		t.Errorf("ARL = %q, want %q", cfg.ARL, "abc123")
	}
}

func TestLoadFromPath_MissingFile(t *testing.T) {
	_, err := loadFromPath(filepath.Join(t.TempDir(), "no-such.toml"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestLoadFromPath_EmptyArl(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("arl = \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := loadFromPath(path)
	if err == nil {
		t.Fatal("expected error for empty arl")
	}
	if !strings.Contains(err.Error(), "arl") {
		t.Errorf("error %q does not mention 'arl'", err)
	}
}

func TestLoadFromPath_LinuxRejectsLaxPermissions(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("permission check is Linux-only")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("arl = \"abc\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadFromPath(path)
	if err == nil {
		t.Fatal("expected error for 0644 permissions")
	}
	if !strings.Contains(err.Error(), "0600") {
		t.Errorf("error %q should mention 0600", err)
	}
}
