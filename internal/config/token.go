package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// GenerateToken returns a 32-byte cryptographically random bearer token
// encoded as base64url without padding (43 chars).
func GenerateToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("rand.Read: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// EnsureToken returns the existing bearer token from cfg, or generates one,
// writes it back to the same TOML file, and returns it. Mutates cfg.BearerToken.
func EnsureToken(path string, cfg *Config) (string, error) {
	if cfg.BearerToken != "" {
		return cfg.BearerToken, nil
	}
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	cfg.BearerToken = tok
	if err := writeConfig(path, cfg); err != nil {
		return "", err
	}
	return tok, nil
}

// RotateToken generates a new bearer token, persists it, and returns it.
func RotateToken(path string, cfg *Config) (string, error) {
	tok, err := GenerateToken()
	if err != nil {
		return "", err
	}
	cfg.BearerToken = tok
	if err := writeConfig(path, cfg); err != nil {
		return "", err
	}
	return tok, nil
}

// DefaultPath returns os.UserConfigDir()/deezer-remote/config.toml.
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("UserConfigDir: %w", err)
	}
	return filepath.Join(base, "deezer-remote", "config.toml"), nil
}

func writeConfig(path string, cfg *Config) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.toml.*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := toml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		return fmt.Errorf("encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}
	if runtime.GOOS == "linux" {
		if err := os.Chmod(tmpPath, 0o600); err != nil {
			return fmt.Errorf("chmod 0600: %w", err)
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
