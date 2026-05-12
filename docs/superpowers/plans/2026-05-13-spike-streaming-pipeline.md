# deezer-remote — Spike (Phase 0) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-track CLI (`cmd/spike`) that proves the Deezer streaming pipeline works end-to-end on Nils's account — given a track ID on stdin, produce a playable MP3 of that track.

**Architecture:** Three internal Go packages (`config`, `gateway`, `media`) developed test-first with faked HTTP transports; a throwaway `cmd/spike/main.go` orchestrator that wires them together and writes the decrypted output to disk. After Phase 0 succeeds, Phase 1 absorbs the three packages and extends them; `cmd/spike` is deleted.

**Tech Stack:** Go 1.25.9, `github.com/BurntSushi/toml` (config), `golang.org/x/crypto/blowfish` (decryption), stdlib `net/http`, `net/http/cookiejar`, `crypto/md5`, `crypto/cipher`, stdlib `flag` for CLI parsing.

**Spec:** `docs/superpowers/specs/2026-05-13-deezer-remote-design.md` — section "Spike" is the source of truth.

---

## Spec coverage

This plan covers **only the Spike** (Section "Spike" in the design doc). Phase 1 (the actual MVP — `serve`, `pair`, `doctor`, web UI, session, transport) is **not** part of this plan; it gets its own plan after the spike findings come back.

The only design-doc requirement the spike does **not** prove is HTTP Range alignment of the decryption stream — that's only needed when `/stream/<id>` exists in Phase 1. The spike downloads each track whole.

## File structure

```
deezer-remote/
  go.mod                                  # module decl + deps
  go.sum                                  # locked deps
  .gitignore                              # Go artifacts + spike outputs
  internal/
    config/
      config.go                           # Load() with 0600 enforcement on Linux
      config_test.go
    gateway/
      client.go                           # Client struct, cookie jar, raw Call
      client_test.go
      csrf.go                             # refreshCSRF, callWithCSRF
      csrf_test.go
      errors.go                           # ErrCSRFExpired, ..., classifyError
      errors_test.go
      tracks.go                           # GetUserData, SongGetData; flexString helper
      tracks_test.go
    media/
      url.go                              # GetURL (media.deezer.com/v1/get_url)
      url_test.go
      crypto.go                           # KeyFromSNGID, Decrypt(r, key) io.Reader
      crypto_test.go
  cmd/
    spike/
      main.go                             # CLI: read --track, run pipeline, write file
```

## Conventions

- **TDD throughout.** Every code-bearing step is preceded by a failing test step.
- **No real network in unit tests.** All HTTP is mocked via a `roundTripFunc` injected into `http.Client.Transport` (see Task 4 step 1). The only live network call is the spike binary itself at Task 12.
- **Module path:** `github.com/niref/deezer-remote`. Adjust import paths accordingly.
- **Commit style:** conventional commits (`feat:`, `test:`, `chore:`). One commit per task unless noted.
- **Working directory** for all commands: `/home/niref/dev/frosco/deezer-remote`.
- **Go version:** 1.25.9 (matches `deezer-tools`).

---

## Task 1: Bootstrap the repository

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Create: `.gitignore`

- [ ] **Step 1: Init module**

Run:
```bash
go mod init github.com/niref/deezer-remote
```

Expected: writes `go.mod` containing `module github.com/niref/deezer-remote` and `go 1.25.9`.

- [ ] **Step 2: Add dependencies**

Run:
```bash
go get github.com/BurntSushi/toml@latest
go get golang.org/x/crypto/blowfish@latest
```

Expected: updates `go.mod` and writes `go.sum`. Both modules pinned.

- [ ] **Step 3: Create .gitignore**

Write `.gitignore`:

```
# Go artifacts
*.exe
*.dll
*.so
*.dylib
*.test
*.out
/dist/
/bin/

# Spike outputs
*.mp3
*.enc
*.mp3.partial
spike-findings-*.txt

# Editor
.idea/
.vscode/
*.swp
```

- [ ] **Step 4: Verify build**

Run:
```bash
go build ./...
```

Expected: no output, exit 0 (nothing to build yet, but module must be valid).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: bootstrap Go module"
```

---

## Task 2: Config package

Reads `arl` from `os.UserConfigDir()/deezer-remote/config.toml`. On Linux, refuses to load if the file mode is more permissive than `0600`. On Windows, skips the mode check.

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/config/...
```

Expected: FAIL — `undefined: loadFromPath`, `undefined: Config`.

- [ ] **Step 3: Write the implementation**

Create `internal/config/config.go`:

```go
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
	ARL string `toml:"arl"`
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/config/...
```

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): load arl from user config dir with 0600 enforcement on Linux"
```

---

## Task 3: Gateway error sentinels and classifier

Defines the error sentinels callers branch on, plus the function that maps gw-light error strings and HTTP statuses onto them. Pure, no transport.

**Files:**
- Create: `internal/gateway/errors.go`
- Create: `internal/gateway/errors_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/gateway/errors_test.go`:

```go
package gateway

import (
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
		want   error
	}{
		{"csrf invalid", "CSRF_TOKEN_INVALID", 200, ErrCSRFExpired},
		{"valid token required", "VALID_TOKEN_REQUIRED", 200, ErrCSRFExpired},
		{"need user auth", "NEED_USER_AUTH_REQUIRED", 200, ErrAuthFailed},
		{"user auth required", "USER_AUTH_REQUIRED", 200, ErrAuthFailed},
		{"data error", "DATA_ERROR", 200, ErrNotFound},
		{"quota error", "QUOTA_ERROR", 200, ErrRateLimited},
		{"http 429", "", 429, ErrRateLimited},
		{"http 500", "", 500, ErrServerError},
		{"http 502", "", 502, ErrServerError},
		{"unknown body", "WEIRD_NEW_ERROR_CODE", 200, ErrUnknown},
		{"http 200 clean", "", 200, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.body, tc.status)
			if tc.want == nil {
				if got != nil {
					t.Errorf("classifyError(%q, %d) = %v, want nil", tc.body, tc.status, got)
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Errorf("classifyError(%q, %d) = %v, want %v", tc.body, tc.status, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/gateway/...
```

Expected: FAIL — `undefined: classifyError`, `undefined: ErrCSRFExpired`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/gateway/errors.go`:

```go
// Package gateway is a thin client for Deezer's unofficial gw-light gateway.
package gateway

import "errors"

var (
	// ErrCSRFExpired means apiToken must be refreshed and the call retried.
	ErrCSRFExpired = errors.New("gateway: CSRF token expired")
	// ErrAuthFailed means the arl is invalid or revoked.
	ErrAuthFailed = errors.New("gateway: auth failed; refresh arl in config")
	// ErrNotFound means the requested resource doesn't exist.
	ErrNotFound = errors.New("gateway: not found")
	// ErrRateLimited means gw-light QUOTA_ERROR or HTTP 429.
	ErrRateLimited = errors.New("gateway: rate limited")
	// ErrServerError means HTTP 5xx from gw-light.
	ErrServerError = errors.New("gateway: server error")
	// ErrUnknown means gw-light returned an error string we don't recognise.
	ErrUnknown = errors.New("gateway: unknown error")
)

// classifyError maps a gw-light error string and HTTP status onto a sentinel.
// Returns nil when there is no error.
func classifyError(body string, status int) error {
	if status >= 500 {
		return ErrServerError
	}
	if status == 429 {
		return ErrRateLimited
	}
	switch body {
	case "":
		if status == 200 {
			return nil
		}
		return ErrUnknown
	case "CSRF_TOKEN_INVALID", "VALID_TOKEN_REQUIRED":
		return ErrCSRFExpired
	case "NEED_USER_AUTH_REQUIRED", "USER_AUTH_REQUIRED":
		return ErrAuthFailed
	case "DATA_ERROR":
		return ErrNotFound
	case "QUOTA_ERROR":
		return ErrRateLimited
	default:
		return ErrUnknown
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/gateway/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/errors.go internal/gateway/errors_test.go
git commit -m "feat(gateway): error sentinels and classifier"
```

---

## Task 4: Gateway Client and raw Call

`Client` holds the `arl`, an HTTP client with a cookie jar (so the server-set `sid` cookie persists across calls), and the current `apiToken`. `Call(ctx, method, params)` issues a POST to gw-light with the configured `apiToken` and returns the raw `results` field of the response. No CSRF refresh logic here — that's Task 5.

**Files:**
- Create: `internal/gateway/client.go`
- Create: `internal/gateway/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/gateway/client_test.go`:

```go
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// roundTripFunc adapts a closure into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mkResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestCall_SendsCorrectRequest(t *testing.T) {
	var seenURL string
	var seenBody string
	var seenCookie string

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenURL = req.URL.String()
		b, _ := io.ReadAll(req.Body)
		seenBody = string(b)
		seenCookie = req.Header.Get("Cookie")
		return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42}}}`), nil
	})

	c, err := newClientWithTransport("ARLVALUE", rt)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := c.Call(context.Background(), "deezer.getUserData", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	// URL must include api_token and method as query params.
	if !strings.Contains(seenURL, "api_version=1.0") {
		t.Errorf("url missing api_version: %s", seenURL)
	}
	if !strings.Contains(seenURL, "method=deezer.getUserData") {
		t.Errorf("url missing method: %s", seenURL)
	}
	if !strings.Contains(seenURL, "api_token=null") {
		t.Errorf("url missing api_token=null for bootstrap call: %s", seenURL)
	}
	if seenBody != "{}" && seenBody != "null" {
		// nil params should serialise to either {} or null; tolerate either.
		t.Errorf("body for nil params = %q", seenBody)
	}
	if !strings.Contains(seenCookie, "arl=ARLVALUE") {
		t.Errorf("cookie missing arl: %q", seenCookie)
	}

	var parsed struct {
		USER struct {
			UserID int `json:"USER_ID"`
		} `json:"USER"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	if parsed.USER.UserID != 42 {
		t.Errorf("USER_ID = %d, want 42", parsed.USER.UserID)
	}
}

func TestCall_ParamsAreEncodedAsJSONBody(t *testing.T) {
	var seenBody string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		seenBody = string(b)
		return mkResp(200, `{"error":[],"results":{}}`), nil
	}) //

	c, _ := newClientWithTransport("ARL", rt)

	_, err := c.Call(context.Background(), "song.getData", map[string]any{"SNG_ID": "3135556"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(seenBody, `"SNG_ID":"3135556"`) {
		t.Errorf("body %q missing SNG_ID", seenBody)
	}
}

func TestCall_ClassifiesGatewayErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		responseJS string
		want       error
	}{
		{
			"csrf invalid in error array",
			200,
			`{"error":{"CSRF_TOKEN_INVALID":"..."},"results":{}}`,
			ErrCSRFExpired,
		},
		{
			"auth failed",
			200,
			`{"error":{"USER_AUTH_REQUIRED":"..."},"results":{}}`,
			ErrAuthFailed,
		},
		{
			"quota error",
			200,
			`{"error":{"QUOTA_ERROR":"..."},"results":{}}`,
			ErrRateLimited,
		},
		{
			"http 500",
			500,
			`internal server error`,
			ErrServerError,
		},
		{
			"http 429",
			429,
			`rate limited`,
			ErrRateLimited,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return mkResp(tc.status, tc.responseJS), nil
			})
			c, _ := newClientWithTransport("ARL", rt)
			_, err := c.Call(context.Background(), "song.getData", nil)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCall_CookieJarPersistsSID(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := mkResp(200, `{"error":[],"results":{}}`)
		if calls == 1 {
			resp.Header.Set("Set-Cookie", "sid=ABC123; Path=/; Domain=deezer.com")
		}
		return resp, nil
	})

	c, _ := newClientWithTransport("ARL", rt)

	if _, err := c.Call(context.Background(), "deezer.getUserData", nil); err != nil {
		t.Fatal(err)
	}

	// Second call must include the sid cookie from the first response.
	var secondCookie string
	rt2 := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		secondCookie = req.Header.Get("Cookie")
		return mkResp(200, `{"error":[],"results":{}}`), nil
	})
	c.http.Transport = rt2
	if _, err := c.Call(context.Background(), "deezer.getUserData", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(secondCookie, "sid=ABC123") {
		t.Errorf("second-call cookie %q does not contain sid=ABC123", secondCookie)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/gateway/...
```

Expected: FAIL — `undefined: newClientWithTransport`, `undefined: Client.Call`.

- [ ] **Step 3: Write the implementation**

Create `internal/gateway/client.go`:

```go
package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

const (
	defaultBaseURL  = "https://www.deezer.com/ajax/gw-light.php"
	defaultAPIVer   = "1.0"
	defaultInput    = "3"
	bootstrapAPITok = "null" // gw-light protocol: literal string "null" for the first call.
)

// Client is the low-level adapter for Deezer's unofficial gw-light gateway.
//
// Cookie jar is required: gw-light binds the CSRF token to the server-set
// `sid` cookie. Replacing the http.Client without preserving the jar will
// break authenticated calls with "Invalid CSRF token".
type Client struct {
	http     *http.Client
	arl      string
	baseURL  string
	apiToken string
}

// NewClient builds a Client with default transport.
func NewClient(arl string) (*Client, error) {
	return newClientWithTransport(arl, http.DefaultTransport)
}

func newClientWithTransport(arl string, rt http.RoundTripper) (*Client, error) {
	if arl == "" {
		return nil, fmt.Errorf("gateway: empty arl")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookiejar: %w", err)
	}
	c := &Client{
		http:     &http.Client{Transport: rt, Jar: jar},
		arl:      arl,
		baseURL:  defaultBaseURL,
		apiToken: bootstrapAPITok,
	}
	// Seed the jar with the arl cookie.
	u, _ := url.Parse(c.baseURL)
	jar.SetCookies(u, []*http.Cookie{{
		Name:   "arl",
		Value:  arl,
		Domain: "deezer.com",
		Path:   "/",
	}})
	return c, nil
}

// Call POSTs `params` to gw-light's `method` endpoint and returns the raw
// `results` JSON. The current apiToken is sent as a query parameter.
//
// Errors are classified via classifyError before returning.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	// gw-light expects null/empty body for nil params; both `{}` and `null` work.
	if string(body) == "null" {
		body = []byte("{}")
	}

	u := fmt.Sprintf(
		"%s?method=%s&input=%s&api_version=%s&api_token=%s",
		c.baseURL,
		url.QueryEscape(method),
		defaultInput,
		defaultAPIVer,
		url.QueryEscape(c.apiToken),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, classifyError("", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// gw-light response envelope: { "error": [] OR { CODE: "msg" }, "results": ... }
	var env struct {
		Error   json.RawMessage `json:"error"`
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}

	if errCode := extractErrorCode(env.Error); errCode != "" {
		return nil, fmt.Errorf("call %s: %w", method, classifyError(errCode, resp.StatusCode))
	}

	return env.Results, nil
}

// extractErrorCode returns the first key of an `error` object, or "" if the
// `error` field is missing, an empty array, or an empty object.
func extractErrorCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try object form: {"CODE":"message"}.
	var asObj map[string]string
	if err := json.Unmarshal(raw, &asObj); err == nil {
		for k := range asObj {
			return k
		}
		return ""
	}
	// Array form (usually empty): [].
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/gateway/...
```

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/client.go internal/gateway/client_test.go
git commit -m "feat(gateway): Client + Call with cookie jar"
```

---

## Task 5: CSRF refresh and callWithCSRF

`refreshCSRF` calls `deezer.getUserData` with `api_token=null` and stores `checkForm` from the response as the new `apiToken`. `callWithCSRF` wraps `Call`: ensures `apiToken` is set, retries once on `ErrCSRFExpired`.

**Files:**
- Create: `internal/gateway/csrf.go`
- Create: `internal/gateway/csrf_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/gateway/csrf_test.go`:

```go
package gateway

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// userDataResp builds a minimal deezer.getUserData success body.
func userDataResp(checkForm string, userID int) string {
	return `{"error":[],"results":{"USER":{"USER_ID":` +
		strconv.Itoa(userID) +
		`,"OPTIONS":{"license_token":"LT"}},"checkForm":"` +
		checkForm +
		`"}}`
}

func TestRefreshCSRF_StoresApiToken(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Bootstrap call must use api_token=null.
		if !strings.Contains(req.URL.RawQuery, "api_token=null") {
			t.Errorf("bootstrap should use api_token=null, got %s", req.URL.RawQuery)
		}
		return mkResp(200, userDataResp("TOK123", 42)), nil
	})

	c, _ := newClientWithTransport("ARL", rt)
	if err := c.refreshCSRF(context.Background()); err != nil {
		t.Fatalf("refreshCSRF: %v", err)
	}
	if c.apiToken != "TOK123" {
		t.Errorf("apiToken = %q, want %q", c.apiToken, "TOK123")
	}
}

func TestRefreshCSRF_UserID0IsAuthFailed(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// USER_ID==0 is gw-light's signal that the arl is invalid.
		return mkResp(200, userDataResp("WHATEVER", 0)), nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	err := c.refreshCSRF(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("err = %v, want ErrAuthFailed", err)
	}
}

func TestCallWithCSRF_BootstrapsThenCalls(t *testing.T) {
	calls := int32(0)
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1:
			// Bootstrap getUserData.
			if !strings.Contains(req.URL.RawQuery, "method=deezer.getUserData") {
				t.Errorf("call 1 should be getUserData, got %s", req.URL.RawQuery)
			}
			return mkResp(200, userDataResp("TOK", 42)), nil
		case 2:
			// Actual method, with refreshed token.
			if !strings.Contains(req.URL.RawQuery, "api_token=TOK") {
				t.Errorf("call 2 should use api_token=TOK, got %s", req.URL.RawQuery)
			}
			if !strings.Contains(req.URL.RawQuery, "method=song.getData") {
				t.Errorf("call 2 should be song.getData, got %s", req.URL.RawQuery)
			}
			return mkResp(200, `{"error":[],"results":{"SNG_ID":"1"}}`), nil
		}
		t.Fatalf("unexpected call %d", n)
		return nil, nil
	})

	c, _ := newClientWithTransport("ARL", rt)
	raw, err := c.callWithCSRF(context.Background(), "song.getData", map[string]any{"SNG_ID": "1"})
	if err != nil {
		t.Fatalf("callWithCSRF: %v", err)
	}
	if !strings.Contains(string(raw), `"SNG_ID":"1"`) {
		t.Errorf("results %s missing SNG_ID", raw)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", got)
	}
}

func TestCallWithCSRF_RetriesOnceOnExpiredCSRF(t *testing.T) {
	calls := int32(0)
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1:
			// Bootstrap.
			return mkResp(200, userDataResp("OLDTOK", 42)), nil
		case 2:
			// First attempt: CSRF expired.
			if !strings.Contains(req.URL.RawQuery, "api_token=OLDTOK") {
				t.Errorf("call 2 should use OLDTOK, got %s", req.URL.RawQuery)
			}
			return mkResp(200, `{"error":{"CSRF_TOKEN_INVALID":"..."},"results":{}}`), nil
		case 3:
			// Re-bootstrap.
			return mkResp(200, userDataResp("NEWTOK", 42)), nil
		case 4:
			// Retry with new token.
			if !strings.Contains(req.URL.RawQuery, "api_token=NEWTOK") {
				t.Errorf("call 4 should use NEWTOK, got %s", req.URL.RawQuery)
			}
			return mkResp(200, `{"error":[],"results":{"ok":true}}`), nil
		}
		t.Fatalf("unexpected call %d", n)
		return nil, nil
	})

	c, _ := newClientWithTransport("ARL", rt)
	_, err := c.callWithCSRF(context.Background(), "song.getData", nil)
	if err != nil {
		t.Fatalf("callWithCSRF: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("expected 4 HTTP calls, got %d", got)
	}
}

func TestCallWithCSRF_DoesNotRetryTwice(t *testing.T) {
	calls := int32(0)
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1, 3:
			return mkResp(200, userDataResp("TOK"+strconv.Itoa(int(n)), 42)), nil
		default:
			// Always expired.
			return mkResp(200, `{"error":{"CSRF_TOKEN_INVALID":"..."},"results":{}}`), nil
		}
	})

	c, _ := newClientWithTransport("ARL", rt)
	_, err := c.callWithCSRF(context.Background(), "song.getData", nil)
	if !errors.Is(err, ErrCSRFExpired) {
		t.Errorf("err = %v, want ErrCSRFExpired after exhausted retry", err)
	}
	// Total: bootstrap(1) + call(2 expired) + refresh(3) + retry(4 expired) = 4.
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("expected 4 HTTP calls, got %d", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/gateway/...
```

Expected: FAIL — `undefined: c.refreshCSRF`, `undefined: c.callWithCSRF`.

- [ ] **Step 3: Write the implementation**

Create `internal/gateway/csrf.go`:

```go
package gateway

import (
	"context"
	"encoding/json"
	"errors"
)

// userDataEnvelope is the subset of deezer.getUserData we care about.
type userDataEnvelope struct {
	CheckForm string `json:"checkForm"`
	User      struct {
		UserID  int `json:"USER_ID"`
		Options struct {
			LicenseToken string `json:"license_token"`
		} `json:"OPTIONS"`
	} `json:"USER"`
}

// refreshCSRF calls deezer.getUserData with api_token=null and updates
// c.apiToken from the response's checkForm field. USER_ID==0 is treated as
// ErrAuthFailed (gw-light's signal that the arl is invalid).
func (c *Client) refreshCSRF(ctx context.Context) error {
	prev := c.apiToken
	c.apiToken = bootstrapAPITok
	raw, err := c.Call(ctx, "deezer.getUserData", nil)
	if err != nil {
		c.apiToken = prev
		return err
	}
	var ud userDataEnvelope
	if err := json.Unmarshal(raw, &ud); err != nil {
		c.apiToken = prev
		return err
	}
	if ud.User.UserID == 0 {
		c.apiToken = prev
		return ErrAuthFailed
	}
	c.apiToken = ud.CheckForm
	return nil
}

// callWithCSRF is the public path for authenticated gw-light methods.
// Ensures apiToken is set (bootstrap if not), and retries once on
// ErrCSRFExpired by refreshing the token.
func (c *Client) callWithCSRF(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.apiToken == "" || c.apiToken == bootstrapAPITok {
		if err := c.refreshCSRF(ctx); err != nil {
			return nil, err
		}
	}

	raw, err := c.Call(ctx, method, params)
	if err == nil {
		return raw, nil
	}
	if !errors.Is(err, ErrCSRFExpired) {
		return nil, err
	}

	// One retry: refresh and try again.
	if rerr := c.refreshCSRF(ctx); rerr != nil {
		return nil, rerr
	}
	return c.Call(ctx, method, params)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/gateway/...
```

Expected: PASS for all CSRF tests plus prior tests.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/csrf.go internal/gateway/csrf_test.go
git commit -m "feat(gateway): CSRF bootstrap and refresh-and-retry"
```

---

## Task 6: Public GetUserData

Public wrapper that calls `refreshCSRF` and returns the parsed user data — specifically the `license_token` we need for `media.getUrl`.

**Files:**
- Create: `internal/gateway/tracks.go` (will also hold `SongGetData` in Task 7)
- Create: `internal/gateway/tracks_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/gateway/tracks_test.go`:

```go
package gateway

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestGetUserData_ReturnsLicenseToken(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42,"OPTIONS":{"license_token":"LIC-TOK"}},"checkForm":"CFTOK"}}`), nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	ud, err := c.GetUserData(context.Background())
	if err != nil {
		t.Fatalf("GetUserData: %v", err)
	}
	if ud.LicenseToken != "LIC-TOK" {
		t.Errorf("LicenseToken = %q, want LIC-TOK", ud.LicenseToken)
	}
	if ud.UserID != 42 {
		t.Errorf("UserID = %d, want 42", ud.UserID)
	}
}

func TestGetUserData_PropagatesAuthFailed(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":0,"OPTIONS":{"license_token":""}},"checkForm":""}}`), nil
	})
	c, _ := newClientWithTransport("BAD-ARL", rt)
	_, err := c.GetUserData(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("err = %v, want ErrAuthFailed", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/gateway/...
```

Expected: FAIL — `undefined: Client.GetUserData`, `undefined: UserData`.

- [ ] **Step 3: Write the implementation**

Create `internal/gateway/tracks.go`:

```go
package gateway

import (
	"context"
	"encoding/json"
)

// UserData is the subset of deezer.getUserData we expose.
type UserData struct {
	UserID       int
	LicenseToken string
}

// GetUserData refreshes the CSRF token and returns the parsed user data.
// USER_ID==0 surfaces as ErrAuthFailed.
func (c *Client) GetUserData(ctx context.Context) (*UserData, error) {
	if err := c.refreshCSRF(ctx); err != nil {
		return nil, err
	}
	// After a successful refresh, do one more call to read fields, because
	// refreshCSRF discards the body. (Alternative: have refreshCSRF return
	// the parsed envelope. For Phase 1 we may refactor; for the spike it's
	// fine to make two calls — the second hits gw-light with a fresh token.)
	raw, err := c.Call(ctx, "deezer.getUserData", nil)
	if err != nil {
		return nil, err
	}
	var ud userDataEnvelope
	if err := json.Unmarshal(raw, &ud); err != nil {
		return nil, err
	}
	if ud.User.UserID == 0 {
		return nil, ErrAuthFailed
	}
	return &UserData{
		UserID:       ud.User.UserID,
		LicenseToken: ud.User.Options.LicenseToken,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/gateway/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/tracks.go internal/gateway/tracks_test.go
git commit -m "feat(gateway): GetUserData public method"
```

---

## Task 7: SongGetData

Calls `song.getData` and returns track metadata. `SNG_ID` is decoded via a flexible string codec because gw-light returns it sometimes quoted and sometimes as a bare number, occasionally within the same response. (This is documented in `deezer-tools`' gateway invariants.)

**Files:**
- Modify: `internal/gateway/tracks.go` (add `SongGetData`, `TrackData`, `flexString`)
- Modify: `internal/gateway/tracks_test.go` (add tests)

- [ ] **Step 1: Append the failing tests**

Append to `internal/gateway/tracks_test.go`:

```go
func TestSongGetData_ReturnsTrackInfo(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case isUserDataCall(req):
			return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42,"OPTIONS":{"license_token":"LT"}},"checkForm":"CF"}}`), nil
		default:
			return mkResp(200, `{"error":[],"results":{"SNG_ID":"3135556","TRACK_TOKEN":"TT-XYZ","MD5_ORIGIN":"abc","MEDIA_VERSION":"7","SNG_TITLE":"Get Lucky","ART_NAME":"Daft Punk"}}`), nil
		}
	})
	c, _ := newClientWithTransport("ARL", rt)
	td, err := c.SongGetData(context.Background(), "3135556")
	if err != nil {
		t.Fatalf("SongGetData: %v", err)
	}
	if td.SngID != "3135556" {
		t.Errorf("SngID = %q, want 3135556", td.SngID)
	}
	if td.TrackToken != "TT-XYZ" {
		t.Errorf("TrackToken = %q", td.TrackToken)
	}
	if td.Title != "Get Lucky" {
		t.Errorf("Title = %q", td.Title)
	}
	if td.Artist != "Daft Punk" {
		t.Errorf("Artist = %q", td.Artist)
	}
}

func TestSongGetData_FlexStringHandlesBareNumber(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case isUserDataCall(req):
			return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42,"OPTIONS":{"license_token":"LT"}},"checkForm":"CF"}}`), nil
		default:
			// SNG_ID as a bare number (no quotes).
			return mkResp(200, `{"error":[],"results":{"SNG_ID":3135556,"TRACK_TOKEN":"TT","MD5_ORIGIN":"a","MEDIA_VERSION":"1","SNG_TITLE":"X","ART_NAME":"Y"}}`), nil
		}
	})
	c, _ := newClientWithTransport("ARL", rt)
	td, err := c.SongGetData(context.Background(), "3135556")
	if err != nil {
		t.Fatalf("SongGetData: %v", err)
	}
	if td.SngID != "3135556" {
		t.Errorf("SngID = %q, want 3135556", td.SngID)
	}
}

// isUserDataCall reports whether req is the deezer.getUserData bootstrap.
func isUserDataCall(req *http.Request) bool {
	return req.URL.Query().Get("method") == "deezer.getUserData"
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/gateway/...
```

Expected: FAIL — `undefined: Client.SongGetData`.

- [ ] **Step 3: Extend the implementation**

Append to `internal/gateway/tracks.go`:

```go
// flexString decodes a JSON field that gw-light returns sometimes as a
// quoted string and sometimes as a bare number.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(string(b))
	return nil
}

// TrackData is the subset of song.getData we expose.
type TrackData struct {
	SngID        string
	TrackToken   string
	MD5Origin    string
	MediaVersion string
	Title        string
	Artist       string
}

type songGetDataEnvelope struct {
	SngID        flexString `json:"SNG_ID"`
	TrackToken   string     `json:"TRACK_TOKEN"`
	MD5Origin    string     `json:"MD5_ORIGIN"`
	MediaVersion flexString `json:"MEDIA_VERSION"`
	Title        string     `json:"SNG_TITLE"`
	Artist       string     `json:"ART_NAME"`
}

// SongGetData fetches metadata for one track via gw-light song.getData.
func (c *Client) SongGetData(ctx context.Context, trackID string) (*TrackData, error) {
	raw, err := c.callWithCSRF(ctx, "song.getData", map[string]any{"SNG_ID": trackID})
	if err != nil {
		return nil, err
	}
	var env songGetDataEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.SngID == "" {
		return nil, ErrNotFound
	}
	return &TrackData{
		SngID:        string(env.SngID),
		TrackToken:   env.TrackToken,
		MD5Origin:    env.MD5Origin,
		MediaVersion: string(env.MediaVersion),
		Title:        env.Title,
		Artist:       env.Artist,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/gateway/...
```

Expected: PASS for all gateway tests.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/tracks.go internal/gateway/tracks_test.go
git commit -m "feat(gateway): SongGetData with flexString SNG_ID decoder"
```

---

## Task 8: Media GetURL

Calls `https://media.deezer.com/v1/get_url` to exchange a track token + license token for a fresh CDN URL. This is a different endpoint from gw-light — separate HTTP client, JSON body, JSON response.

> **Note for the agent:** The exact request/response shape below is the canonical scheme used by deemix / d-fi as of 2024. The spike (Task 12) verifies it against live Deezer; if the shape differs, update the JSON tags and the test response payloads accordingly and re-run.

**Files:**
- Create: `internal/media/url.go`
- Create: `internal/media/url_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/media/url_test.go`:

```go
package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGetURL_SendsRightBodyAndParsesResponse(t *testing.T) {
	var seenBody map[string]any
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://media.deezer.com/v1/get_url" {
			t.Errorf("URL = %q", req.URL)
		}
		if req.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", req.Method)
		}
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &seenBody); err != nil {
			t.Fatalf("body not JSON: %s", body)
		}
		respBody := `{"data":[{"media":[{"media_type":"FULL","cipher":{"type":"BF_CBC_STRIPE"},"format":"MP3_320","sources":[{"url":"https://cdn.example/track.bin","provider":"x"}],"nbf":1000,"exp":2000}]}]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(respBody)),
			Header:     make(http.Header),
		}, nil
	})

	c := NewClient(rt)
	res, err := c.GetURL(context.Background(), URLRequest{
		LicenseToken: "LIC",
		TrackToken:   "TT-XYZ",
		Formats:      []Format{FormatMP3_320, FormatMP3_128},
	})
	if err != nil {
		t.Fatalf("GetURL: %v", err)
	}
	if res.URL != "https://cdn.example/track.bin" {
		t.Errorf("URL = %q", res.URL)
	}
	if res.Format != FormatMP3_320 {
		t.Errorf("Format = %q, want MP3_320", res.Format)
	}
	if res.Expiry != 2000 {
		t.Errorf("Expiry = %d, want 2000", res.Expiry)
	}

	// Body shape sanity.
	if seenBody["license_token"] != "LIC" {
		t.Errorf("body.license_token = %v", seenBody["license_token"])
	}
	tks, ok := seenBody["track_tokens"].([]any)
	if !ok || len(tks) != 1 || tks[0] != "TT-XYZ" {
		t.Errorf("body.track_tokens = %v", seenBody["track_tokens"])
	}
}

func TestGetURL_EmptyDataIsNotAvailable(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"errors":[{"code":2002,"message":"unavailable"}]}]}`)),
			Header:     make(http.Header),
		}, nil
	})
	c := NewClient(rt)
	_, err := c.GetURL(context.Background(), URLRequest{
		LicenseToken: "LIC",
		TrackToken:   "TT",
		Formats:      []Format{FormatMP3_128},
	})
	if err == nil {
		t.Fatal("expected error for empty media array")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/media/...
```

Expected: FAIL — `undefined: NewClient`, `undefined: Format`, etc.

- [ ] **Step 3: Write the implementation**

Create `internal/media/url.go`:

```go
// Package media handles Deezer's track-streaming endpoints: stream URL
// acquisition (media.getUrl) and on-the-fly Blowfish decryption.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const mediaGetURLEndpoint = "https://media.deezer.com/v1/get_url"

// Format names the audio formats we may request.
type Format string

const (
	FormatMP3_128 Format = "MP3_128"
	FormatMP3_320 Format = "MP3_320"
)

// URLRequest is the input to GetURL.
type URLRequest struct {
	LicenseToken string
	TrackToken   string
	Formats      []Format // preference order; first is preferred
}

// URLResult is the chosen CDN URL plus metadata.
type URLResult struct {
	URL    string
	Format Format
	Expiry int64 // unix seconds when the URL expires
}

// Client speaks to media.deezer.com/v1/get_url.
type Client struct {
	http *http.Client
}

// NewClient builds a Client with the given transport.
func NewClient(rt http.RoundTripper) *Client {
	return &Client{http: &http.Client{Transport: rt}}
}

type urlReqWire struct {
	LicenseToken string         `json:"license_token"`
	TrackTokens  []string       `json:"track_tokens"`
	Media        []mediaSpec    `json:"media"`
}

type mediaSpec struct {
	Type    string          `json:"type"`
	Formats []formatSpec    `json:"formats"`
}

type formatSpec struct {
	Cipher string `json:"cipher"`
	Format string `json:"format"`
}

type urlRespWire struct {
	Data []struct {
		Media []struct {
			MediaType string `json:"media_type"`
			Cipher    struct {
				Type string `json:"type"`
			} `json:"cipher"`
			Format  string `json:"format"`
			Sources []struct {
				URL string `json:"url"`
			} `json:"sources"`
			Expiry int64 `json:"exp"`
		} `json:"media"`
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"data"`
}

// GetURL exchanges a track token + license token for a fresh CDN URL.
func (c *Client) GetURL(ctx context.Context, in URLRequest) (*URLResult, error) {
	formats := make([]formatSpec, 0, len(in.Formats))
	for _, f := range in.Formats {
		formats = append(formats, formatSpec{Cipher: "BF_CBC_STRIPE", Format: string(f)})
	}

	wire := urlReqWire{
		LicenseToken: in.LicenseToken,
		TrackTokens:  []string{in.TrackToken},
		Media: []mediaSpec{{
			Type:    "FULL",
			Formats: formats,
		}},
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediaGetURLEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post get_url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("get_url: http %d", resp.StatusCode)
	}

	var w urlRespWire
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil {
		return nil, fmt.Errorf("decode get_url response: %w", err)
	}

	if len(w.Data) == 0 {
		return nil, fmt.Errorf("get_url: empty data")
	}
	if len(w.Data[0].Errors) > 0 {
		e := w.Data[0].Errors[0]
		return nil, fmt.Errorf("get_url: track unavailable (code %d: %s)", e.Code, e.Message)
	}
	if len(w.Data[0].Media) == 0 {
		return nil, fmt.Errorf("get_url: no media returned (no format match for tier?)")
	}
	m := w.Data[0].Media[0]
	if len(m.Sources) == 0 {
		return nil, fmt.Errorf("get_url: media has no sources")
	}
	return &URLResult{
		URL:    m.Sources[0].URL,
		Format: Format(m.Format),
		Expiry: m.Expiry,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/media/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/url.go internal/media/url_test.go
git commit -m "feat(media): GetURL client for media.deezer.com/v1/get_url"
```

---

## Task 9: Blowfish key derivation

`KeyFromSNGID(sngID)` returns the 16-byte Blowfish key derived from `md5(sngID)` XOR'd with the known secret `g4el58wc0zvf9na1`. Pure function; trivially testable.

**Files:**
- Create: `internal/media/crypto.go`
- Create: `internal/media/crypto_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/media/crypto_test.go`:

```go
package media

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"testing"
)

func TestKeyFromSNGID_KnownVector(t *testing.T) {
	// SNG_ID = "1" -> md5("1") = "c4ca4238a0b923820dcc509a6f75849b" (lowercase hex).
	// Algorithm: key[i] = md5_hex[i] XOR md5_hex[i+16] XOR secret[i] (XOR of byte values).
	got := KeyFromSNGID("1")

	// Independent derivation for the test: compute it inline.
	want := derivedKey(t, "1")
	if !bytes.Equal(got[:], want[:]) {
		t.Errorf("KeyFromSNGID(\"1\") = %x, want %x", got, want)
	}
}

func TestKeyFromSNGID_Deterministic(t *testing.T) {
	a := KeyFromSNGID("3135556")
	b := KeyFromSNGID("3135556")
	if a != b {
		t.Error("KeyFromSNGID not deterministic")
	}
}

// derivedKey is the reference implementation re-derived inside the test, so
// the test does not just compare the function against itself.
func derivedKey(t *testing.T, sngID string) [16]byte {
	t.Helper()
	sum := md5.Sum([]byte(sngID))
	hexed := hex.EncodeToString(sum[:]) // 32 lowercase hex chars
	const secret = "g4el58wc0zvf9na1"
	var k [16]byte
	for i := 0; i < 16; i++ {
		k[i] = hexed[i] ^ hexed[i+16] ^ secret[i]
	}
	return k
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/media/...
```

Expected: FAIL — `undefined: KeyFromSNGID`.

- [ ] **Step 3: Write the implementation**

Create `internal/media/crypto.go`:

```go
package media

import (
	"crypto/md5"
	"encoding/hex"
)

// blowfishSecret is the 16-char XOR mask used by Deezer's key derivation.
// Documented in deemix/d-fi; not a placeholder.
const blowfishSecret = "g4el58wc0zvf9na1"

// KeyFromSNGID derives the 16-byte Blowfish-CBC key for a given track.
//
//	key[i] = md5_hex[i] XOR md5_hex[i+16] XOR secret[i]
//
// where md5_hex is the lowercase-hex MD5 of the SNG_ID string and secret is
// the known 16-char constant. XOR is on byte values of the ASCII chars,
// not on the bytes the hex represents.
func KeyFromSNGID(sngID string) [16]byte {
	sum := md5.Sum([]byte(sngID))
	hexed := hex.EncodeToString(sum[:])
	var k [16]byte
	for i := 0; i < 16; i++ {
		k[i] = hexed[i] ^ hexed[i+16] ^ blowfishSecret[i]
	}
	return k
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/media/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/crypto.go internal/media/crypto_test.go
git commit -m "feat(media): KeyFromSNGID Blowfish key derivation"
```

---

## Task 10: Decryption stream (the stride)

`Decrypt(r io.Reader, key [16]byte) io.Reader` returns a reader that consumes encrypted bytes and emits decrypted bytes. The scheme: process input in 2048-byte chunks; every block at byte offset divisible by 6144 (i.e., block indices 0, 3, 6, 9, ...) is Blowfish-CBC decrypted with IV `0x0001020304050607`; the rest passes through unchanged. Partial final block passes through.

> **Stride note:** "every 6144-th byte" = "every 3rd 2048-byte block" because 6144 = 3 * 2048. The spec doc phrases it the first way; the code phrases it as `if blockIndex % 3 == 0`. They're equivalent. The spike will catch it if the canonical implementation has drifted.

**Files:**
- Modify: `internal/media/crypto.go`
- Modify: `internal/media/crypto_test.go`

- [ ] **Step 1: Append the failing tests**

First, replace the import block at the top of `internal/media/crypto_test.go` with the merged set (the new entries are `crypto/cipher`, `io`, and `golang.org/x/crypto/blowfish`):

```go
import (
	"bytes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"io"
	"testing"

	"golang.org/x/crypto/blowfish"
)
```

Then append the following test functions at the end of the file:

```go
func TestDecrypt_StrideRespected(t *testing.T) {
	const blockSize = 2048
	const numBlocks = 9 // exercises three encrypted-block positions: 0, 3, 6

	key := [16]byte{}
	for i := range key {
		key[i] = byte(i + 1) // arbitrary non-zero key
	}

	// Build 9 plaintext blocks. Blocks 0/3/6 will be encrypted; rest passthrough.
	plain := make([]byte, numBlocks*blockSize)
	for i := 0; i < numBlocks*blockSize; i++ {
		plain[i] = byte(i % 251) // deterministic non-trivial pattern
	}

	// Encrypt blocks 0, 3, 6 in place to build the "encrypted stream".
	encStream := make([]byte, len(plain))
	copy(encStream, plain)

	bc, err := blowfish.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	for blk := 0; blk < numBlocks; blk++ {
		if blk%3 != 0 {
			continue
		}
		start := blk * blockSize
		mode := cipher.NewCBCEncrypter(bc, iv)
		mode.CryptBlocks(encStream[start:start+blockSize], plain[start:start+blockSize])
	}

	// Now Decrypt should recover `plain` from `encStream`.
	r := Decrypt(bytesReader(encStream), key)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(plain) {
		t.Fatalf("len = %d, want %d", len(got), len(plain))
	}
	for i := range plain {
		if got[i] != plain[i] {
			t.Fatalf("mismatch at byte %d (block %d, offset %d): got %02x want %02x", i, i/blockSize, i%blockSize, got[i], plain[i])
		}
	}
}

func TestDecrypt_PartialFinalBlockPassthrough(t *testing.T) {
	const blockSize = 2048
	key := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	// One full passthrough block (index 1, not 0) + 500 trailing bytes.
	// Encrypted positions are i%3==0, so index 1 is passthrough.
	stream := make([]byte, blockSize+500)
	for i := range stream {
		stream[i] = byte(i % 251)
	}
	// Insert one encrypted block at index 0 by pre-pending.
	// To keep this test simple, just check that bytes after position 0 pass through.
	// We use only blocks 1 + a partial — index 0 will exist and be "encrypted",
	// but we control its plaintext to be all-zeros so encryption produces ciphertext
	// that, when decrypted, yields zeros — predictable.
	allZeros := make([]byte, blockSize)
	bc, _ := blowfish.NewCipher(key[:])
	enc0 := make([]byte, blockSize)
	cipher.NewCBCEncrypter(bc, []byte{0, 1, 2, 3, 4, 5, 6, 7}).CryptBlocks(enc0, allZeros)

	encStream := append([]byte{}, enc0...)
	encStream = append(encStream, stream...)

	r := Decrypt(bytesReader(encStream), key)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(encStream) {
		t.Fatalf("len = %d, want %d", len(got), len(encStream))
	}
	// First block should now be all zeros.
	for i := 0; i < blockSize; i++ {
		if got[i] != 0 {
			t.Fatalf("block 0 byte %d = %02x, want 0", i, got[i])
		}
	}
	// Trailing 500 bytes should be untouched (passthrough as part of the
	// partial final block).
	for i := 0; i < 500; i++ {
		want := byte((blockSize + i) % 251)
		if got[blockSize+blockSize+i] != want {
			t.Fatalf("trailing byte %d = %02x, want %02x", i, got[blockSize+blockSize+i], want)
		}
	}
}

// bytesReader wraps a []byte as an io.Reader that reads up to a chunked size
// per call, to expose buffering bugs in Decrypt.
func bytesReader(b []byte) io.Reader {
	return &chunkedReader{buf: b, chunk: 137} // arbitrary non-aligned chunk
}

type chunkedReader struct {
	buf   []byte
	pos   int
	chunk int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.buf) {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(p) {
		n = len(p)
	}
	if n > len(r.buf)-r.pos {
		n = len(r.buf) - r.pos
	}
	copy(p, r.buf[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
go test ./internal/media/...
```

Expected: FAIL — `undefined: Decrypt`.

- [ ] **Step 3: Extend the implementation**

First, replace the import block at the top of `internal/media/crypto.go` with the merged set (new entries: `crypto/cipher`, `io`, `golang.org/x/crypto/blowfish`):

```go
import (
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/blowfish"
)
```

Then append the following declarations at the end of the file:

```go
const (
	blockSize        = 2048
	encryptionStride = 3 // every 3rd block is encrypted
)

var blowfishIV = []byte{0, 1, 2, 3, 4, 5, 6, 7}

// Decrypt returns a reader that consumes Deezer's partially-encrypted byte
// stream and emits the plaintext audio bytes.
//
// Layout: input is split into 2048-byte blocks. Blocks at index 0, 3, 6, ...
// are Blowfish-CBC ciphertexts (key from KeyFromSNGID, IV 0x0001020304050607).
// All other blocks are passthrough. A final partial block (< 2048 bytes) is
// always passthrough.
func Decrypt(r io.Reader, key [16]byte) io.Reader {
	return &decryptReader{
		src: r,
		key: key,
	}
}

type decryptReader struct {
	src      io.Reader
	key      [16]byte
	bc       cipher.Block
	bcOnce   bool
	buf      []byte // bytes ready to be emitted to the consumer
	blockIdx int    // index of the next block to process
	srcEOF   bool   // src returned io.EOF on its last full block read
}

func (d *decryptReader) Read(p []byte) (int, error) {
	if len(d.buf) == 0 && !d.srcEOF {
		if err := d.fillNextBlock(); err != nil && len(d.buf) == 0 {
			return 0, err
		}
	}
	if len(d.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

// fillNextBlock pulls one block (up to 2048 bytes) from src, processes it,
// and appends the result to d.buf.
func (d *decryptReader) fillNextBlock() error {
	blk := make([]byte, blockSize)
	n, err := io.ReadFull(d.src, blk)
	switch {
	case err == nil:
		// Full block.
		out, perr := d.processBlock(blk[:n], true)
		if perr != nil {
			return perr
		}
		d.buf = append(d.buf, out...)
		d.blockIdx++
		return nil
	case err == io.ErrUnexpectedEOF || err == io.EOF:
		// Partial final block — always passthrough per the Deezer scheme.
		d.srcEOF = true
		if n > 0 {
			d.buf = append(d.buf, blk[:n]...)
		}
		return io.EOF
	default:
		return err
	}
}

// processBlock returns the output bytes for one full input block. If the
// current block is at an encrypted-stride position, decrypt; else passthrough.
func (d *decryptReader) processBlock(in []byte, full bool) ([]byte, error) {
	if !full || d.blockIdx%encryptionStride != 0 {
		// Passthrough — copy so callers see independent bytes.
		out := make([]byte, len(in))
		copy(out, in)
		return out, nil
	}
	if !d.bcOnce {
		bc, err := blowfish.NewCipher(d.key[:])
		if err != nil {
			return nil, err
		}
		d.bc = bc
		d.bcOnce = true
	}
	out := make([]byte, len(in))
	cipher.NewCBCDecrypter(d.bc, blowfishIV).CryptBlocks(out, in)
	return out, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
go test ./internal/media/...
```

Expected: PASS for both decryption tests plus the key-derivation tests.

- [ ] **Step 5: Commit**

```bash
git add internal/media/crypto.go internal/media/crypto_test.go
git commit -m "feat(media): Decrypt with 6144-byte stride and partial-tail passthrough"
```

---

## Task 11: Spike CLI

The throwaway orchestrator. Reads `--track <id>`, loads config, runs the full pipeline, writes `./<id>.mp3` to disk. Logs each phase to stderr. On any failure, dumps the encrypted bytes to `./<id>.enc` and any partial decrypted output to `./<id>.mp3.partial` to aid debugging.

**Files:**
- Create: `cmd/spike/main.go`

- [ ] **Step 1: Skim — no unit test for the orchestrator**

Rationale: this is a glue script and its real test is Task 12 (running it against live Deezer). We do not add a unit test here because the meaningful failure modes (wrong protocol, wrong key, wrong stride) are exactly what Task 12 catches, and unit-testing the orchestrator would re-test the packages we already covered.

- [ ] **Step 2: Write the implementation**

Create `cmd/spike/main.go`:

```go
// Command spike proves the Deezer streaming pipeline end-to-end on Nils's
// account. Throwaway: deleted after Phase 1 starts.
//
//	$ deezer-remote-spike --track 3135556
//	playing: Daft Punk - Get Lucky (MP3_320)
//	got CDN url (TTL 1m48s), file size 8.4 MB
//	decrypted in 1.2s
//	wrote ./3135556.mp3
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
)

func main() {
	var trackID string
	flag.StringVar(&trackID, "track", "", "Deezer track ID to fetch (required)")
	flag.Parse()
	if trackID == "" {
		log.Fatal("--track is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, trackID); err != nil {
		log.Fatalf("spike: %v", err)
	}
}

func run(ctx context.Context, trackID string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	gw, err := gateway.NewClient(cfg.ARL)
	if err != nil {
		return fmt.Errorf("gateway: %w", err)
	}

	fmt.Fprintln(os.Stderr, "→ fetching user data")
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		return fmt.Errorf("getUserData: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  user_id=%d license_token=%s…\n", ud.UserID, truncate(ud.LicenseToken, 12))

	fmt.Fprintln(os.Stderr, "→ song.getData")
	td, err := gw.SongGetData(ctx, trackID)
	if err != nil {
		return fmt.Errorf("song.getData: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  %s — %s (sng_id=%s)\n", td.Artist, td.Title, td.SngID)

	fmt.Fprintln(os.Stderr, "→ media.getUrl (MP3_320 then MP3_128)")
	mc := media.NewClient(http.DefaultTransport)
	mr, err := mc.GetURL(ctx, media.URLRequest{
		LicenseToken: ud.LicenseToken,
		TrackToken:   td.TrackToken,
		Formats:      []media.Format{media.FormatMP3_320, media.FormatMP3_128},
	})
	if err != nil {
		return fmt.Errorf("media.getUrl: %w", err)
	}
	fmt.Fprintf(os.Stderr, "  format=%s url=%s… exp=%d\n", mr.Format, truncate(mr.URL, 60), mr.Expiry)

	// Fetch encrypted bytes.
	fmt.Fprintln(os.Stderr, "→ GET CDN url")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mr.URL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("cdn get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("cdn get: http %d", resp.StatusCode)
	}
	fmt.Fprintf(os.Stderr, "  content-length=%d\n", resp.ContentLength)

	// Save raw encrypted bytes as a debugging artefact.
	encPath := filepath.Clean(trackID + ".enc")
	encFile, err := os.Create(encPath)
	if err != nil {
		return err
	}
	defer encFile.Close()

	mp3Path := filepath.Clean(trackID + ".mp3")
	mp3File, err := os.Create(mp3Path)
	if err != nil {
		return err
	}
	defer mp3File.Close()

	// Pipe: HTTP body → tee → (encFile, decryptReader → mp3File).
	tee := io.TeeReader(resp.Body, encFile)

	key := media.KeyFromSNGID(td.SngID)
	dec := media.Decrypt(tee, key)

	start := time.Now()
	n, err := io.Copy(mp3File, dec)
	if err != nil {
		// Best-effort: keep the partial output.
		_ = os.Rename(mp3Path, mp3Path+".partial")
		return fmt.Errorf("decrypt copy: %w", err)
	}
	fmt.Fprintf(os.Stderr, "→ decrypted %d bytes in %s\n", n, time.Since(start).Round(time.Millisecond))

	// Sanity: very rough MP3-header check on first byte (0xFF) — purely informational.
	header := make([]byte, 4)
	if f, err := os.Open(mp3Path); err == nil {
		if _, err := io.ReadFull(f, header); err == nil {
			frameSync := binary.BigEndian.Uint16(header[0:2]) & 0xFFE0
			if frameSync == 0xFFE0 {
				fmt.Fprintln(os.Stderr, "  ✓ output starts with MP3 frame sync")
			} else {
				fmt.Fprintf(os.Stderr, "  ⚠ output does not start with MP3 frame sync (first bytes: % x)\n", header)
			}
		}
		f.Close()
	}

	// Drop the .enc on success — it's only useful for failures.
	_ = encFile.Close()
	_ = os.Remove(encPath)

	fmt.Fprintf(os.Stderr, "wrote %s\n", mp3Path)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
```

- [ ] **Step 3: Verify it builds**

Run:
```bash
go build ./cmd/spike
```

Expected: no output, exit 0. (If imports fail, fix and re-run.)

- [ ] **Step 4: Run static analysis**

Run:
```bash
go vet ./...
```

Expected: no output, exit 0.

- [ ] **Step 5: Commit**

```bash
git add cmd/spike/main.go
git commit -m "feat(spike): cmd/spike orchestrator"
```

---

## Task 12: Live run + findings note

The real test. Run the spike against Nils's account with a track ID. Verify the output file is a playable MP3. Append a findings paragraph to the design doc.

**Files:**
- Modify: `docs/superpowers/specs/2026-05-13-deezer-remote-design.md` (append findings)

**Pre-requisite:** `~/.config/deezer-remote/config.toml` exists, mode `0600`, contains `arl = "..."`. If missing:

```bash
mkdir -p ~/.config/deezer-remote
printf 'arl = "PASTE_ARL_HERE"\n' > ~/.config/deezer-remote/config.toml
chmod 0600 ~/.config/deezer-remote/config.toml
```

(How to get an arl: log in at https://www.deezer.com, DevTools → Application → Cookies → `https://www.deezer.com` → copy `arl` value.)

- [ ] **Step 1: Pick a track ID**

Nils picks a track ID he's currently entitled to play. Easiest source: open any track on deezer.com, the URL is `https://www.deezer.com/track/<ID>`. Suggested defaults if Nils wants a known-public one: `3135556` (Daft Punk – Get Lucky).

- [ ] **Step 2: Run the spike**

Run:
```bash
go run ./cmd/spike --track <TRACK_ID> 2>&1 | tee spike-findings-$(date -u +%Y%m%dT%H%M%SZ).txt
```

Expected: stderr stream of phase markers ending with `wrote <id>.mp3`. The `.txt` file captures stderr for the findings note.

- [ ] **Step 3: Play the output**

Run:
```bash
ffplay -nodisp -autoexit <TRACK_ID>.mp3   # if ffmpeg is installed
# or open the file with any audio player.
```

Expected: the track plays. Audio is the full track, not corrupted.

- [ ] **Step 4: Try the lower-quality fallback (informational)**

Edit `cmd/spike/main.go` to request only `media.FormatMP3_128` (comment out MP3_320), rebuild, and run against the same track. Compares whether MP3_320 was actually returned or whether the account silently fell back. **Revert the edit before committing.**

```bash
go run ./cmd/spike --track <TRACK_ID>
```

Note in your findings whether the URL changed and whether the played output is audibly lower quality.

- [ ] **Step 5: Capture the findings note**

Append the following section to `docs/superpowers/specs/2026-05-13-deezer-remote-design.md` (inside the `## Spike` section, right before `## Error handling`):

```markdown
### Findings (filled in after the live run)

**Date:** YYYY-MM-DD
**Track:** `<TRACK_ID>` (`<Artist - Title>`)

- MP3_320 returned: ✓ / ✗
- MP3_128 returned: ✓ / ✗
- FLAC attempted: not in spike (deferred)
- Media URL TTL (`exp` minus `nbf`, in seconds): N
- Headers required beyond defaults: (none observed / list them)
- Region/availability behavior for an inaccessible track ID: (describe the error shape)
- Time from `--track` to `wrote <id>.mp3`: N seconds
- Anomalies observed: (none / describe)

**Verdict:** GO / NO-GO for Phase 1 design. (If NO-GO, summary of what broke and which design assumption it invalidates.)
```

Fill in the values from the actual run. If the verdict is NO-GO, **stop** — Phase 1 plan needs the spec revised before being written.

- [ ] **Step 6: Commit the findings**

```bash
git add docs/superpowers/specs/2026-05-13-deezer-remote-design.md
git commit -m "spec(spike): record findings from live run"
```

- [ ] **Step 7: Clean up temporary files**

```bash
rm -f *.mp3 *.enc *.mp3.partial spike-findings-*.txt
```

These are gitignored but worth removing from the working tree.

---

## Done

After Task 12 commits a GO verdict, the spike is complete. The next step is **writing a new plan for Phase 1** (the MVP), which absorbs `internal/config`, `internal/gateway`, `internal/media` and adds `internal/session`, `internal/transport`, `internal/web`, and `cmd/deezer-remote`. `cmd/spike/` is deleted as part of Phase 1's first commit.

If the verdict is NO-GO, revise the spec's player-model decision and re-brainstorm.

## Self-review notes

- **Spec coverage:** Every requirement in the spec's `## Spike` section maps to a task: pipeline steps 1–7 → Tasks 2 through 11; success criteria → Task 12; "questions the spike answers" → captured in the findings template (Task 12 step 5); failure handling (write `.enc` + `.partial`) → Task 11 step 2.
- **Placeholder scan:** No "TBD"/"TODO" in steps. Two informational notes flag that protocol shapes may differ — these are not placeholders, they're verification hooks for Task 12.
- **Type consistency:** `Format`, `URLRequest`, `URLResult`, `TrackData`, `UserData`, `Client` (gateway and media — different packages), `KeyFromSNGID([16]byte)`, `Decrypt(io.Reader, [16]byte) io.Reader` consistent across tasks.
- **One-task-one-commit** held throughout.
