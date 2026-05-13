package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateToken_IsBase64URL32Bytes(t *testing.T) {
	tok, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	// Base64url(32 bytes, no padding) = 43 chars.
	if len(tok) != 43 {
		t.Errorf("len(token) = %d, want 43", len(tok))
	}
	if strings.ContainsAny(tok, "+/=") {
		t.Errorf("token %q contains non-base64url chars", tok)
	}
	tok2, _ := GenerateToken()
	if tok == tok2 {
		t.Errorf("two generated tokens should differ")
	}
}

func TestLoadOrInitToken_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Seed with arl-only config.
	if err := os.WriteFile(path, []byte(`arl = "ARLVALUE"`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "" {
		t.Fatal("preconditions: BearerToken should be empty before init")
	}
	tok, err := EnsureToken(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" || len(tok) != 43 {
		t.Errorf("token = %q", tok)
	}
	// File now contains both arl and bearer_token.
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "ARLVALUE") {
		t.Error("arl was clobbered")
	}
	if !strings.Contains(string(got), tok) {
		t.Error("bearer_token not persisted")
	}
}

func TestLoadOrInitToken_ReturnsExistingToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `arl = "X"` + "\n" + `bearer_token = "existing-token-value"` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BearerToken != "existing-token-value" {
		t.Errorf("BearerToken = %q", cfg.BearerToken)
	}
	tok, err := EnsureToken(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tok != "existing-token-value" {
		t.Errorf("EnsureToken returned %q, want existing-token-value", tok)
	}
}

func TestRotateToken_OverwritesAndReturnsNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `arl = "X"` + "\n" + `bearer_token = "old"` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	newTok, err := RotateToken(path, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if newTok == "old" || len(newTok) != 43 {
		t.Errorf("newTok = %q", newTok)
	}
	got, _ := os.ReadFile(path)
	if strings.Contains(string(got), `"old"`) {
		t.Error("old token still present")
	}
	if !strings.Contains(string(got), newTok) {
		t.Error("new token not persisted")
	}
}
