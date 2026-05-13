// Package config loads the arl cookie from the user's config directory.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk config shape.
type Config struct {
	ARL         string `toml:"arl"`
	BearerToken string `toml:"bearer_token"`
}

// Load reads ~/.config/deezer-remote/config.toml on Linux,
// %APPDATA%\deezer-remote\config.toml on Windows.
//
// On Linux, refuses if mode is more permissive than 0600.
// On Windows, relies on %APPDATA% being user-private by default ACLs.
func Load() (*Config, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("UserConfigDir: %w", err)
	}
	return loadFromPath(filepath.Join(base, "deezer-remote", "config.toml"))
}

// LoadFromPath reads a config file at an explicit path. Used by the pair
// and doctor subcommands so they can operate on the same file Load uses.
func LoadFromPath(path string) (*Config, error) { return loadFromPath(path) }

func loadFromPath(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if runtime.GOOS == "linux" {
		if mode := info.Mode().Perm(); mode&0o077 != 0 {
			return nil, fmt.Errorf("config %s has permission %04o, expected 0600 (run: chmod 0600 %s)", path, mode, path)
		}
	}
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.ARL == "" {
		return nil, errors.New("config has empty or missing 'arl' field")
	}
	return &cfg, nil
}
