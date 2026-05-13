# deezer-remote Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a single-user, on-LAN remote-control music app. Phone (controller) searches and queues tracks; laptop browser (player) plays decrypted MP3 from Deezer; service is authoritative for session state and proxies the encrypted stream.

**Architecture:** Bottom-up build that respects the spec's strict layering (`cmd → session → media → gateway`; `transport → session`). Foundations (`internal/config`, `internal/gateway`, `internal/media`) exist from the spike; this plan extends them, adds `internal/session` and `internal/transport`, builds the Alpine.js + plain-JS web UI under `web/dist/`, and wires three Cobra subcommands. Reference spec: `docs/superpowers/specs/2026-05-13-deezer-remote-design.md`.

**Tech Stack:**
- Go 1.25, `BurntSushi/toml` (existing), `golang.org/x/crypto/blowfish` (existing).
- New deps: `github.com/spf13/cobra` (CLI), `github.com/coder/websocket` (WS), `github.com/mdp/qrterminal/v3` (terminal QR).
- Browser: Alpine.js 3.x (vendored, no Node toolchain), plain JS modules.
- Tests: stdlib `testing`, `httptest`, `roundTripFunc` pattern already used in `internal/gateway`.

**Conventions used in this plan:**
- File paths are absolute from repo root.
- Commit messages use Conventional Commits (`feat:`, `test:`, `chore:`, `docs:`).
- After each task: `go build ./... && go test ./...` must pass before committing.
- `go mod tidy` runs whenever deps change, and is part of the same commit as the code that uses them.
- File mode for new Go files: standard `gofmt`.
- Engineer must not skip the "run the failing test" step — verifying RED before GREEN catches typos in test names.

---

## File Map

```
deezer-remote/
  cmd/
    deezer-remote/
      main.go              # cobra root, version, wires subcommands           [Task 24]
      serve.go             # serve subcommand                                  [Task 26]
      pair.go              # pair subcommand                                   [Task 25]
      doctor.go            # doctor subcommand                                 [Task 27]
    spike/                 # DELETED in Task 29
  internal/
    config/
      config.go            # existing — load arl                               [unchanged]
      token.go             # bearer token generate/load/persist                [Task 1]
      token_test.go                                                            [Task 1]
    gateway/
      client.go, csrf.go, errors.go, tracks.go (existing)
      tracks.go            # extend TrackData with FileSizeMP3_320/_128        [Task 2]
      search.go            # deezer.pageSearch                                 [Task 3]
      search_test.go                                                           [Task 3]
      album.go             # album.getData + song.getListByAlbum               [Task 4]
      album_test.go                                                            [Task 4]
      playlist.go          # playlist.getData + playlist.getSongs              [Task 5]
      playlist_test.go                                                         [Task 5]
  internal/
    media/
      crypto.go, url.go (existing)
      cache.go             # URL+metadata cache, TTL aware                     [Task 6]
      cache_test.go                                                            [Task 6]
      resolver.go          # cached track_id → resolved bundle                 [Task 7]
      resolver_test.go                                                         [Task 7]
      stream.go            # range-aware streaming reader                      [Task 8]
      stream_test.go                                                           [Task 8]
    session/
      track.go             # shared Track type                                 [Task 9]
      session.go           # state, transport intent                           [Task 9]
      session_test.go                                                          [Task 9]
      advance.go           # auto-advance with 5-skip cap                      [Task 10]
      advance_test.go                                                          [Task 10]
    network/
      lan.go               # LAN IP enumeration                                [Task 11]
      lan_test.go                                                              [Task 11]
    qrterm/
      qrterm.go            # terminal QR wrapper                               [Task 12]
    transport/
      messages.go          # WS wire types + classified Error                  [Task 13]
      auth.go              # bearer token middleware                           [Task 14]
      auth_test.go                                                             [Task 14]
      api.go               # /api/search, /api/track, /api/album, /api/playlist [Task 15]
      api_test.go                                                              [Task 15]
      stream.go            # /stream/<id> HTTP handler                         [Task 16]
      stream_test.go                                                           [Task 16]
      hub.go               # WS hub: role negotiation, fan-out, heartbeat      [Task 17]
      hub_test.go                                                              [Task 17]
      router.go            # command routing → session                         [Task 18]
      router_test.go                                                           [Task 18]
      server.go            # HTTP+WS server assembly                           [Task 19]
    web/
      embed.go             # //go:embed dist                                   [Task 20]
  web/
    dist/
      index.html           # single-page shell, Alpine attributes              [Task 20]
      styles.css           # mockup-fidelity dark styling                      [Task 20]
      vendor/alpine.min.js # Alpine 3.x vendored                               [Task 20]
      js/api.js            # /api/* fetch helpers                              [Task 21]
      js/ws.js             # WebSocket client w/ reconnect                     [Task 21]
      js/stores.js         # Alpine stores: session, ui                        [Task 21]
      js/player.js         # <audio> control + playback push                   [Task 22]
      js/controller.js     # search, queue, transport actions                  [Task 23]
      js/app.js            # bootstrap, role selection                         [Task 23]
  docs/
    superpowers/
      specs/2026-05-13-deezer-remote-design.md (existing)
      plans/2026-05-14-deezer-remote-phase1.md  (this file)
```

---

## Phase A — Extended foundations

### Task 1: Bearer token in config

**Files:**
- Create: `internal/config/token.go`
- Create: `internal/config/token_test.go`
- Modify: `internal/config/config.go` (extend `Config` struct)

- [ ] **Step 1: Write the failing tests**

```go
// internal/config/token_test.go
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
	_ = os.WriteFile(path, []byte(body), 0o600)
	cfg, _ := loadFromPath(path)
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
```

- [ ] **Step 2: Run the failing tests**

Run: `go test ./internal/config/ -run TestGenerateToken -v`
Expected: FAIL — `undefined: GenerateToken` (and other undefined symbols).

- [ ] **Step 3: Extend `Config` and implement token helpers**

Edit `internal/config/config.go` — add `BearerToken` field. Replace the `Config` struct:

```go
// Config is the on-disk config shape.
type Config struct {
	ARL         string `toml:"arl"`
	BearerToken string `toml:"bearer_token"`
}
```

Change `loadFromPath` so it no longer errors on a missing `bearer_token` (it's allowed to be empty until `EnsureToken` runs):

```go
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

Create `internal/config/token.go`:

```go
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
		return "", err
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
		return err
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
```

- [ ] **Step 4: Run the tests; verify they pass**

Run: `go test ./internal/config/ -v`
Expected: all PASS.

- [ ] **Step 5: Run `go build ./...` and full test suite**

Run: `go build ./... && go test ./...`
Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add internal/config/
git commit -m "feat(config): bearer token generate / load / persist

Adds BearerToken to Config, atomic-rename TOML writer, EnsureToken
and RotateToken helpers, and DefaultPath. Used by the pair / serve
subcommands and by transport's bearer-token middleware."
```

---

### Task 2: Extend `TrackData` with format file sizes

The streaming proxy needs `Content-Length` for the chosen format. `song.getData` returns `FILESIZE_MP3_320` and `FILESIZE_MP3_128`. Capture them in `TrackData`.

**Files:**
- Modify: `internal/gateway/tracks.go`
- Modify: `internal/gateway/tracks_test.go`

- [ ] **Step 1: Add the test**

Append to `internal/gateway/tracks_test.go`:

```go
func TestSongGetData_ReturnsFileSizes(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "method=deezer.getUserData") {
			return mkResp(200, `{"error":[],"results":{"checkForm":"TOK","USER":{"USER_ID":1,"OPTIONS":{"license_token":"L"}}}}`), nil
		}
		body := `{"error":[],"results":{"SNG_ID":"42","TRACK_TOKEN":"TT","MD5_ORIGIN":"abc","MEDIA_VERSION":"4","SNG_TITLE":"T","ART_NAME":"A","FILESIZE_MP3_320":"9059264","FILESIZE_MP3_128":"3623705","DURATION":"226","ALB_TITLE":"Discovery","ALB_PICTURE":"md5pic"}}`
		return mkResp(200, body), nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	td, err := c.SongGetData(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if td.FileSizeMP3_320 != 9059264 {
		t.Errorf("FileSizeMP3_320 = %d", td.FileSizeMP3_320)
	}
	if td.FileSizeMP3_128 != 3623705 {
		t.Errorf("FileSizeMP3_128 = %d", td.FileSizeMP3_128)
	}
	if td.DurationS != 226 {
		t.Errorf("DurationS = %d", td.DurationS)
	}
	if td.Album != "Discovery" {
		t.Errorf("Album = %q", td.Album)
	}
	if td.CoverMD5 != "md5pic" {
		t.Errorf("CoverMD5 = %q", td.CoverMD5)
	}
}
```

- [ ] **Step 2: Run, expect FAIL** — `undefined: FileSizeMP3_320` and friends.

Run: `go test ./internal/gateway/ -run TestSongGetData_ReturnsFileSizes -v`

- [ ] **Step 3: Extend `TrackData` and decoder**

In `internal/gateway/tracks.go`:

```go
// TrackData is the subset of song.getData we expose.
type TrackData struct {
	SngID           string
	TrackToken      string
	MD5Origin       string
	MediaVersion    string
	Title           string
	Artist          string
	Album           string
	CoverMD5        string // ALB_PICTURE; build URL via image CDN
	DurationS       int
	FileSizeMP3_320 int64
	FileSizeMP3_128 int64
}

type songGetDataEnvelope struct {
	SngID           flexString `json:"SNG_ID"`
	TrackToken      string     `json:"TRACK_TOKEN"`
	MD5Origin       string     `json:"MD5_ORIGIN"`
	MediaVersion    flexString `json:"MEDIA_VERSION"`
	Title           string     `json:"SNG_TITLE"`
	Artist          string     `json:"ART_NAME"`
	Album           string     `json:"ALB_TITLE"`
	CoverMD5        string     `json:"ALB_PICTURE"`
	Duration        flexString `json:"DURATION"`
	FileSizeMP3_320 flexString `json:"FILESIZE_MP3_320"`
	FileSizeMP3_128 flexString `json:"FILESIZE_MP3_128"`
}
```

Replace the construction in `SongGetData`:

```go
	return &TrackData{
		SngID:           string(env.SngID),
		TrackToken:      env.TrackToken,
		MD5Origin:       env.MD5Origin,
		MediaVersion:    string(env.MediaVersion),
		Title:           env.Title,
		Artist:          env.Artist,
		Album:           env.Album,
		CoverMD5:        env.CoverMD5,
		DurationS:       atoiOrZero(string(env.Duration)),
		FileSizeMP3_320: atoi64OrZero(string(env.FileSizeMP3_320)),
		FileSizeMP3_128: atoi64OrZero(string(env.FileSizeMP3_128)),
	}, nil
```

Add the helper to `internal/gateway/tracks.go` (at the bottom):

```go
func atoiOrZero(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func atoi64OrZero(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
```

Add `"strconv"` to the imports.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/gateway/ -v`
Expected: all PASS (including pre-existing tests).

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/tracks.go internal/gateway/tracks_test.go
git commit -m "feat(gateway): expose FILESIZE_MP3_{320,128}, DURATION, album, cover

Phase 1 needs Content-Length for /stream/<id> and album/cover metadata
for the UI. Wire decoder captures the additional fields; flexString
handles gw-light's string-vs-number inconsistency."
```

---

### Task 3: `gateway.Search` via `deezer.pageSearch`

**Files:**
- Create: `internal/gateway/search.go`
- Create: `internal/gateway/search_test.go`

`deezer.pageSearch` returns top results across TRACK / ALBUM / PLAYLIST categories. We surface only what the UI needs.

- [ ] **Step 1: Write the failing test**

```go
// internal/gateway/search_test.go
package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSearch_ParsesTracksAlbumsPlaylists(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.String(), "method=deezer.getUserData") {
			return mkResp(200, `{"error":[],"results":{"checkForm":"TOK","USER":{"USER_ID":1,"OPTIONS":{"license_token":"L"}}}}`), nil
		}
		body := `{"error":[],"results":{
			"TRACK":{"data":[
				{"SNG_ID":"1","SNG_TITLE":"Get Lucky","ART_NAME":"Daft Punk","ALB_TITLE":"RAM","ALB_PICTURE":"pic1","DURATION":"248"}
			],"count":1},
			"ALBUM":{"data":[
				{"ALB_ID":"10","ALB_TITLE":"RAM","ART_NAME":"Daft Punk","ALB_PICTURE":"pic1","NUMBER_TRACK":"13"}
			],"count":1},
			"PLAYLIST":{"data":[
				{"PLAYLIST_ID":"100","TITLE":"Daft Punk Essentials","PARENT_USERNAME":"deezer","PLAYLIST_PICTURE":"pic2","NB_SONG":"42"}
			],"count":1}
		}}`
		return mkResp(200, body), nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	res, err := c.Search(context.Background(), "daft punk", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tracks) != 1 || res.Tracks[0].ID != "1" || res.Tracks[0].Artist != "Daft Punk" {
		t.Errorf("Tracks = %+v", res.Tracks)
	}
	if len(res.Albums) != 1 || res.Albums[0].ID != "10" || res.Albums[0].TrackCount != 13 {
		t.Errorf("Albums = %+v", res.Albums)
	}
	if len(res.Playlists) != 1 || res.Playlists[0].ID != "100" || res.Playlists[0].TrackCount != 42 {
		t.Errorf("Playlists = %+v", res.Playlists)
	}
}

func TestSearch_EmptyQueryErrors(t *testing.T) {
	c, _ := newClientWithTransport("ARL", roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("transport should not be called")
		return nil, nil
	}))
	if _, err := c.Search(context.Background(), "  ", 10); err == nil {
		t.Error("expected error for empty query")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/gateway/ -run TestSearch -v`

- [ ] **Step 3: Implement**

Create `internal/gateway/search.go`:

```go
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// SearchResult is the parsed payload for /api/search.
type SearchResult struct {
	Tracks    []TrackSummary    `json:"tracks"`
	Albums    []AlbumSummary    `json:"albums"`
	Playlists []PlaylistSummary `json:"playlists"`
}

// TrackSummary is a track in search results / album / playlist listings.
type TrackSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	CoverMD5  string `json:"cover_md5"`
	DurationS int    `json:"duration_s"`
}

// AlbumSummary is an album in search results.
type AlbumSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	CoverMD5   string `json:"cover_md5"`
	TrackCount int    `json:"track_count"`
}

// PlaylistSummary is a playlist in search results.
type PlaylistSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Owner      string `json:"owner"`
	CoverMD5   string `json:"cover_md5"`
	TrackCount int    `json:"track_count"`
}

type searchWire struct {
	Track struct {
		Data []struct {
			SngID     flexString `json:"SNG_ID"`
			Title     string     `json:"SNG_TITLE"`
			Artist    string     `json:"ART_NAME"`
			Album     string     `json:"ALB_TITLE"`
			AlbPic    string     `json:"ALB_PICTURE"`
			Duration  flexString `json:"DURATION"`
		} `json:"data"`
	} `json:"TRACK"`
	Album struct {
		Data []struct {
			AlbID    flexString `json:"ALB_ID"`
			Title    string     `json:"ALB_TITLE"`
			Artist   string     `json:"ART_NAME"`
			AlbPic   string     `json:"ALB_PICTURE"`
			NumTrack flexString `json:"NUMBER_TRACK"`
		} `json:"data"`
	} `json:"ALBUM"`
	Playlist struct {
		Data []struct {
			PlaylistID flexString `json:"PLAYLIST_ID"`
			Title      string     `json:"TITLE"`
			Owner      string     `json:"PARENT_USERNAME"`
			Pic        string     `json:"PLAYLIST_PICTURE"`
			NbSong     flexString `json:"NB_SONG"`
		} `json:"data"`
	} `json:"PLAYLIST"`
}

// Search queries gw-light deezer.pageSearch and returns the parsed payload.
func (c *Client) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("gateway: empty search query")
	}
	if limit <= 0 {
		limit = 20
	}
	raw, err := c.callWithCSRF(ctx, "deezer.pageSearch", map[string]any{
		"query":          q,
		"start":          0,
		"nb":             limit,
		"suggest":        true,
		"artist_suggest": false,
		"top_tracks":     true,
	})
	if err != nil {
		return nil, err
	}
	var w searchWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, err
	}
	out := &SearchResult{
		Tracks:    make([]TrackSummary, 0, len(w.Track.Data)),
		Albums:    make([]AlbumSummary, 0, len(w.Album.Data)),
		Playlists: make([]PlaylistSummary, 0, len(w.Playlist.Data)),
	}
	for _, t := range w.Track.Data {
		out.Tracks = append(out.Tracks, TrackSummary{
			ID:        string(t.SngID),
			Title:     t.Title,
			Artist:    t.Artist,
			Album:     t.Album,
			CoverMD5:  t.AlbPic,
			DurationS: atoiOrZero(string(t.Duration)),
		})
	}
	for _, a := range w.Album.Data {
		out.Albums = append(out.Albums, AlbumSummary{
			ID:         string(a.AlbID),
			Title:      a.Title,
			Artist:     a.Artist,
			CoverMD5:   a.AlbPic,
			TrackCount: atoiOrZero(string(a.NumTrack)),
		})
	}
	for _, p := range w.Playlist.Data {
		out.Playlists = append(out.Playlists, PlaylistSummary{
			ID:         string(p.PlaylistID),
			Title:      p.Title,
			Owner:      p.Owner,
			CoverMD5:   p.Pic,
			TrackCount: atoiOrZero(string(p.NbSong)),
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/gateway/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/search.go internal/gateway/search_test.go
git commit -m "feat(gateway): Search via deezer.pageSearch

Returns TrackSummary / AlbumSummary / PlaylistSummary trimmed to what
the UI needs. flexString already handles gw-light's number/string
inconsistency for IDs, durations, and counts."
```

---

### Task 4: `gateway.Album` — `album.getData` + `song.getListByAlbum`

**Files:**
- Create: `internal/gateway/album.go`
- Create: `internal/gateway/album_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/gateway/album_test.go
package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestAlbum_ReturnsHeaderAndTracks(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		url := req.URL.String()
		if strings.Contains(url, "method=deezer.getUserData") {
			return mkResp(200, `{"error":[],"results":{"checkForm":"TOK","USER":{"USER_ID":1,"OPTIONS":{"license_token":"L"}}}}`), nil
		}
		if strings.Contains(url, "method=album.getData") {
			return mkResp(200, `{"error":[],"results":{
				"ALB_ID":"10","ALB_TITLE":"RAM","ART_NAME":"Daft Punk","ALB_PICTURE":"pic1","NUMBER_TRACK":"13"
			}}`), nil
		}
		if strings.Contains(url, "method=song.getListByAlbum") {
			return mkResp(200, `{"error":[],"results":{"data":[
				{"SNG_ID":"1","SNG_TITLE":"Give Life Back to Music","ART_NAME":"Daft Punk","ALB_TITLE":"RAM","ALB_PICTURE":"pic1","DURATION":"275"},
				{"SNG_ID":"2","SNG_TITLE":"Get Lucky","ART_NAME":"Daft Punk","ALB_TITLE":"RAM","ALB_PICTURE":"pic1","DURATION":"248"}
			]}}`), nil
		}
		t.Fatalf("unexpected URL: %s", url)
		return nil, nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	alb, err := c.Album(context.Background(), "10")
	if err != nil {
		t.Fatal(err)
	}
	if alb.Header.ID != "10" || alb.Header.Title != "RAM" {
		t.Errorf("Header = %+v", alb.Header)
	}
	if len(alb.Tracks) != 2 || alb.Tracks[0].ID != "1" || alb.Tracks[1].ID != "2" {
		t.Errorf("Tracks = %+v", alb.Tracks)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/gateway/ -run TestAlbum -v`

- [ ] **Step 3: Implement**

Create `internal/gateway/album.go`:

```go
package gateway

import (
	"context"
	"encoding/json"
)

// Album is an album header plus its full tracklist.
type Album struct {
	Header AlbumSummary   `json:"header"`
	Tracks []TrackSummary `json:"tracks"`
}

type albumHeaderWire struct {
	AlbID    flexString `json:"ALB_ID"`
	Title    string     `json:"ALB_TITLE"`
	Artist   string     `json:"ART_NAME"`
	AlbPic   string     `json:"ALB_PICTURE"`
	NumTrack flexString `json:"NUMBER_TRACK"`
}

type albumTracksWire struct {
	Data []struct {
		SngID    flexString `json:"SNG_ID"`
		Title    string     `json:"SNG_TITLE"`
		Artist   string     `json:"ART_NAME"`
		Album    string     `json:"ALB_TITLE"`
		AlbPic   string     `json:"ALB_PICTURE"`
		Duration flexString `json:"DURATION"`
	} `json:"data"`
}

// Album fetches the album header and full tracklist in order.
func (c *Client) Album(ctx context.Context, albumID string) (*Album, error) {
	rawHeader, err := c.callWithCSRF(ctx, "album.getData", map[string]any{"ALB_ID": albumID})
	if err != nil {
		return nil, err
	}
	var h albumHeaderWire
	if err := json.Unmarshal(rawHeader, &h); err != nil {
		return nil, err
	}
	if h.AlbID == "" {
		return nil, ErrNotFound
	}

	rawTracks, err := c.callWithCSRF(ctx, "song.getListByAlbum", map[string]any{
		"ALB_ID": albumID,
		"start":  0,
		"nb":     -1,
	})
	if err != nil {
		return nil, err
	}
	var tw albumTracksWire
	if err := json.Unmarshal(rawTracks, &tw); err != nil {
		return nil, err
	}

	out := &Album{
		Header: AlbumSummary{
			ID:         string(h.AlbID),
			Title:      h.Title,
			Artist:     h.Artist,
			CoverMD5:   h.AlbPic,
			TrackCount: atoiOrZero(string(h.NumTrack)),
		},
		Tracks: make([]TrackSummary, 0, len(tw.Data)),
	}
	for _, t := range tw.Data {
		out.Tracks = append(out.Tracks, TrackSummary{
			ID:        string(t.SngID),
			Title:     t.Title,
			Artist:    t.Artist,
			Album:     t.Album,
			CoverMD5:  t.AlbPic,
			DurationS: atoiOrZero(string(t.Duration)),
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/gateway/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/album.go internal/gateway/album_test.go
git commit -m "feat(gateway): Album fetches header + full tracklist"
```

---

### Task 5: `gateway.Playlist` — `playlist.getData` + `playlist.getSongs`

**Files:**
- Create: `internal/gateway/playlist.go`
- Create: `internal/gateway/playlist_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/gateway/playlist_test.go
package gateway

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestPlaylist_ReturnsHeaderAndTracks(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		url := req.URL.String()
		if strings.Contains(url, "method=deezer.getUserData") {
			return mkResp(200, `{"error":[],"results":{"checkForm":"TOK","USER":{"USER_ID":1,"OPTIONS":{"license_token":"L"}}}}`), nil
		}
		if strings.Contains(url, "method=playlist.getData") {
			return mkResp(200, `{"error":[],"results":{
				"PLAYLIST_ID":"100","TITLE":"My Mix","PARENT_USERNAME":"nils","PLAYLIST_PICTURE":"pic","NB_SONG":"2"
			}}`), nil
		}
		if strings.Contains(url, "method=playlist.getSongs") {
			return mkResp(200, `{"error":[],"results":{"data":[
				{"SNG_ID":"1","SNG_TITLE":"A","ART_NAME":"X","ALB_TITLE":"Z","ALB_PICTURE":"p","DURATION":"180"},
				{"SNG_ID":"2","SNG_TITLE":"B","ART_NAME":"X","ALB_TITLE":"Z","ALB_PICTURE":"p","DURATION":"200"}
			]}}`), nil
		}
		t.Fatalf("unexpected URL: %s", url)
		return nil, nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	p, err := c.Playlist(context.Background(), "100")
	if err != nil {
		t.Fatal(err)
	}
	if p.Header.ID != "100" || p.Header.Owner != "nils" {
		t.Errorf("Header = %+v", p.Header)
	}
	if len(p.Tracks) != 2 || p.Tracks[1].ID != "2" {
		t.Errorf("Tracks = %+v", p.Tracks)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/gateway/ -run TestPlaylist -v`

- [ ] **Step 3: Implement**

Create `internal/gateway/playlist.go`:

```go
package gateway

import (
	"context"
	"encoding/json"
)

// Playlist is a playlist header plus its tracklist.
type Playlist struct {
	Header PlaylistSummary `json:"header"`
	Tracks []TrackSummary  `json:"tracks"`
}

type playlistHeaderWire struct {
	PlaylistID flexString `json:"PLAYLIST_ID"`
	Title      string     `json:"TITLE"`
	Owner      string     `json:"PARENT_USERNAME"`
	Pic        string     `json:"PLAYLIST_PICTURE"`
	NbSong     flexString `json:"NB_SONG"`
}

type playlistSongsWire struct {
	Data []struct {
		SngID    flexString `json:"SNG_ID"`
		Title    string     `json:"SNG_TITLE"`
		Artist   string     `json:"ART_NAME"`
		Album    string     `json:"ALB_TITLE"`
		AlbPic   string     `json:"ALB_PICTURE"`
		Duration flexString `json:"DURATION"`
	} `json:"data"`
}

// Playlist fetches the playlist header and tracklist.
func (c *Client) Playlist(ctx context.Context, playlistID string) (*Playlist, error) {
	rawHeader, err := c.callWithCSRF(ctx, "playlist.getData", map[string]any{"PLAYLIST_ID": playlistID})
	if err != nil {
		return nil, err
	}
	var h playlistHeaderWire
	if err := json.Unmarshal(rawHeader, &h); err != nil {
		return nil, err
	}
	if h.PlaylistID == "" {
		return nil, ErrNotFound
	}

	rawSongs, err := c.callWithCSRF(ctx, "playlist.getSongs", map[string]any{
		"PLAYLIST_ID": playlistID,
		"start":       0,
		"nb":          -1,
	})
	if err != nil {
		return nil, err
	}
	var sw playlistSongsWire
	if err := json.Unmarshal(rawSongs, &sw); err != nil {
		return nil, err
	}

	out := &Playlist{
		Header: PlaylistSummary{
			ID:         string(h.PlaylistID),
			Title:      h.Title,
			Owner:      h.Owner,
			CoverMD5:   h.Pic,
			TrackCount: atoiOrZero(string(h.NbSong)),
		},
		Tracks: make([]TrackSummary, 0, len(sw.Data)),
	}
	for _, t := range sw.Data {
		out.Tracks = append(out.Tracks, TrackSummary{
			ID:        string(t.SngID),
			Title:     t.Title,
			Artist:    t.Artist,
			Album:     t.Album,
			CoverMD5:  t.AlbPic,
			DurationS: atoiOrZero(string(t.Duration)),
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/gateway/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/gateway/playlist.go internal/gateway/playlist_test.go
git commit -m "feat(gateway): Playlist fetches header + tracklist"
```

---

## Phase B — Media layer

### Task 6: URL + metadata cache

The cache maps `track_id → ResolvedTrack` (CDN URL, expiry, format, sng_id, size). Decisions doc says "refetch when remaining TTL < 30 min; evict on 4xx".

**Files:**
- Create: `internal/media/cache.go`
- Create: `internal/media/cache_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/media/cache_test.go
package media

import (
	"testing"
	"time"
)

func TestCache_PutGet(t *testing.T) {
	c := NewCache(30 * time.Minute)
	now := time.Unix(1_000_000, 0)
	c.clock = func() time.Time { return now }

	r := ResolvedTrack{
		TrackID:  "42",
		SngID:    "42",
		URL:      "https://cdn/x",
		Format:   FormatMP3_320,
		Size:     9059264,
		ExpiryAt: now.Add(2 * time.Hour),
	}
	c.Put(r)
	got, ok := c.Get("42")
	if !ok || got.URL != "https://cdn/x" {
		t.Errorf("Get = %+v, ok=%v", got, ok)
	}
}

func TestCache_RefetchWhenInsideSafetyMargin(t *testing.T) {
	c := NewCache(30 * time.Minute)
	now := time.Unix(1_000_000, 0)
	c.clock = func() time.Time { return now }
	c.Put(ResolvedTrack{TrackID: "42", URL: "u", ExpiryAt: now.Add(29 * time.Minute)})

	_, ok := c.Get("42")
	if ok {
		t.Errorf("entry expiring in 29 min must be considered stale (margin=30m)")
	}
	// And evicted.
	c.Put(ResolvedTrack{TrackID: "42", URL: "u2", ExpiryAt: now.Add(31 * time.Minute)})
	if got, ok := c.Get("42"); !ok || got.URL != "u2" {
		t.Errorf("Get after re-put = %+v, ok=%v", got, ok)
	}
}

func TestCache_Evict(t *testing.T) {
	c := NewCache(30 * time.Minute)
	c.Put(ResolvedTrack{TrackID: "42", URL: "u", ExpiryAt: time.Now().Add(10 * time.Hour)})
	c.Evict("42")
	if _, ok := c.Get("42"); ok {
		t.Error("Evict failed")
	}
}

func TestCache_ConcurrentSafe(t *testing.T) {
	c := NewCache(30 * time.Minute)
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				c.Put(ResolvedTrack{TrackID: "x", URL: "u", ExpiryAt: time.Now().Add(time.Hour)})
				_, _ = c.Get("x")
				c.Evict("x")
			}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/media/ -run TestCache -v`

- [ ] **Step 3: Implement**

Create `internal/media/cache.go`:

```go
package media

import (
	"sync"
	"time"
)

// ResolvedTrack is everything /stream/<id> needs to serve the track without
// re-running song.getData / media.getUrl.
type ResolvedTrack struct {
	TrackID  string
	SngID    string
	URL      string
	Format   Format
	Size     int64
	ExpiryAt time.Time
}

// Cache is an in-memory, goroutine-safe map of track_id → ResolvedTrack.
// Entries are considered stale when remaining TTL falls below SafetyMargin.
type Cache struct {
	mu           sync.Mutex
	entries      map[string]ResolvedTrack
	SafetyMargin time.Duration
	clock        func() time.Time
}

// NewCache builds an empty cache. safetyMargin is the minimum remaining TTL
// an entry must have to be considered fresh.
func NewCache(safetyMargin time.Duration) *Cache {
	return &Cache{
		entries:      make(map[string]ResolvedTrack),
		SafetyMargin: safetyMargin,
		clock:        time.Now,
	}
}

// Put stores or replaces the entry for r.TrackID.
func (c *Cache) Put(r ResolvedTrack) {
	c.mu.Lock()
	c.entries[r.TrackID] = r
	c.mu.Unlock()
}

// Get returns the entry for trackID if it exists and is still fresh.
// Stale entries are deleted as a side effect.
func (c *Cache) Get(trackID string) (ResolvedTrack, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.entries[trackID]
	if !ok {
		return ResolvedTrack{}, false
	}
	if r.ExpiryAt.Sub(c.clock()) < c.SafetyMargin {
		delete(c.entries, trackID)
		return ResolvedTrack{}, false
	}
	return r, true
}

// Evict removes the entry for trackID. Safe on a missing key.
func (c *Cache) Evict(trackID string) {
	c.mu.Lock()
	delete(c.entries, trackID)
	c.mu.Unlock()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/media/ -v`
Expected: all PASS, including `-race`.

Run also: `go test -race ./internal/media/ -run TestCache_ConcurrentSafe`

- [ ] **Step 5: Commit**

```bash
git add internal/media/cache.go internal/media/cache_test.go
git commit -m "feat(media): in-memory URL cache with safety-margin TTL

Goroutine-safe map track_id -> ResolvedTrack. Entries are considered
stale when remaining TTL falls below SafetyMargin (30 min for serve).
Stale Get auto-evicts to keep the map honest."
```

---

### Task 7: `Resolver` — cached `track_id → ResolvedTrack`

The Resolver glues `gateway.SongGetData`, `media.URLClient.GetURL`, and `Cache`. It exposes a single `Resolve(ctx, trackID)` method.

**Files:**
- Create: `internal/media/resolver.go`
- Create: `internal/media/resolver_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/media/resolver_test.go
package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeTrackFetcher implements the resolver's dependencies via closures.
type fakeTrackFetcher struct {
	songGetData func(ctx context.Context, trackID string) (TrackInfo, error)
	getURL      func(ctx context.Context, req URLRequest) (*URLResult, error)
}

func (f fakeTrackFetcher) SongGetData(ctx context.Context, trackID string) (TrackInfo, error) {
	return f.songGetData(ctx, trackID)
}
func (f fakeTrackFetcher) GetURL(ctx context.Context, req URLRequest) (*URLResult, error) {
	return f.getURL(ctx, req)
}

func TestResolver_Resolve_FetchesAndCaches(t *testing.T) {
	songCalls, urlCalls := 0, 0
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			songCalls++
			return TrackInfo{SngID: "42", TrackToken: "TT", FileSizeMP3_320: 9_000_000, FileSizeMP3_128: 3_000_000}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			urlCalls++
			return &URLResult{URL: "https://cdn/x", Format: FormatMP3_320, Expiry: time.Now().Add(20 * time.Hour).Unix()}, nil
		},
	}
	r := NewResolver(f, "LIC", []Format{FormatMP3_320, FormatMP3_128}, NewCache(30*time.Minute))

	rt1, err := r.Resolve(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if rt1.Format != FormatMP3_320 || rt1.Size != 9_000_000 {
		t.Errorf("first resolve = %+v", rt1)
	}
	rt2, err := r.Resolve(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if rt2.URL != rt1.URL {
		t.Errorf("cache miss on 2nd resolve")
	}
	if songCalls != 1 || urlCalls != 1 {
		t.Errorf("expected 1 song, 1 url; got song=%d url=%d", songCalls, urlCalls)
	}
}

func TestResolver_PicksSizeMatchingFormat(t *testing.T) {
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			return TrackInfo{SngID: "42", TrackToken: "TT", FileSizeMP3_320: 9_000_000, FileSizeMP3_128: 3_000_000}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			return &URLResult{URL: "u", Format: FormatMP3_128, Expiry: time.Now().Add(20 * time.Hour).Unix()}, nil
		},
	}
	r := NewResolver(f, "LIC", []Format{FormatMP3_320, FormatMP3_128}, NewCache(30*time.Minute))
	got, err := r.Resolve(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if got.Size != 3_000_000 {
		t.Errorf("Size = %d, want FileSizeMP3_128", got.Size)
	}
}

func TestResolver_RefetchesAfterEvict(t *testing.T) {
	calls := 0
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			return TrackInfo{SngID: "42", TrackToken: "TT", FileSizeMP3_320: 1, FileSizeMP3_128: 1}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			calls++
			return &URLResult{URL: "u", Format: FormatMP3_320, Expiry: time.Now().Add(20 * time.Hour).Unix()}, nil
		},
	}
	cache := NewCache(30 * time.Minute)
	r := NewResolver(f, "LIC", []Format{FormatMP3_320}, cache)
	if _, err := r.Resolve(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	cache.Evict("42")
	if _, err := r.Resolve(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected 2 getURL calls after evict; got %d", calls)
	}
}

func TestResolver_PropagatesNotAvailable(t *testing.T) {
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			return TrackInfo{SngID: "42", TrackToken: "TT"}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			return nil, errors.New("get_url: track unavailable (code 2002: blocked)")
		},
	}
	r := NewResolver(f, "LIC", []Format{FormatMP3_320}, NewCache(time.Minute))
	if _, err := r.Resolve(context.Background(), "42"); err == nil {
		t.Error("expected error to propagate")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/media/ -run TestResolver -v`

- [ ] **Step 3: Implement**

Create `internal/media/resolver.go`:

```go
package media

import (
	"context"
	"fmt"
	"time"
)

// TrackInfo is the subset of song.getData the resolver needs. The gateway
// package provides a TrackData that satisfies this shape; we declare a local
// type so internal/media stays independent of internal/gateway (strict
// layering: media must not import gateway).
type TrackInfo struct {
	SngID           string
	TrackToken      string
	FileSizeMP3_320 int64
	FileSizeMP3_128 int64
}

// TrackFetcher abstracts the two outbound calls the resolver makes. The
// transport-layer wiring code provides an adapter over gateway.Client and
// media.URLClient.
type TrackFetcher interface {
	SongGetData(ctx context.Context, trackID string) (TrackInfo, error)
	GetURL(ctx context.Context, req URLRequest) (*URLResult, error)
}

// Resolver turns a track_id into a ResolvedTrack, using a Cache to skip the
// outbound calls when a fresh entry is available.
type Resolver struct {
	f            TrackFetcher
	licenseToken string
	formats      []Format
	cache        *Cache
}

// NewResolver wires a resolver. formats is the preference order passed to
// media.getUrl (typically [MP3_320, MP3_128]).
func NewResolver(f TrackFetcher, licenseToken string, formats []Format, cache *Cache) *Resolver {
	return &Resolver{f: f, licenseToken: licenseToken, formats: formats, cache: cache}
}

// Resolve returns a fresh, ready-to-stream ResolvedTrack.
func (r *Resolver) Resolve(ctx context.Context, trackID string) (ResolvedTrack, error) {
	if hit, ok := r.cache.Get(trackID); ok {
		return hit, nil
	}
	ti, err := r.f.SongGetData(ctx, trackID)
	if err != nil {
		return ResolvedTrack{}, fmt.Errorf("song.getData(%s): %w", trackID, err)
	}
	res, err := r.f.GetURL(ctx, URLRequest{
		LicenseToken: r.licenseToken,
		TrackToken:   ti.TrackToken,
		Formats:      r.formats,
	})
	if err != nil {
		return ResolvedTrack{}, fmt.Errorf("media.getUrl(%s): %w", trackID, err)
	}
	size := sizeForFormat(ti, res.Format)
	out := ResolvedTrack{
		TrackID:  trackID,
		SngID:    ti.SngID,
		URL:      res.URL,
		Format:   res.Format,
		Size:     size,
		ExpiryAt: time.Unix(res.Expiry, 0),
	}
	r.cache.Put(out)
	return out, nil
}

func sizeForFormat(ti TrackInfo, f Format) int64 {
	switch f {
	case FormatMP3_320:
		return ti.FileSizeMP3_320
	case FormatMP3_128:
		return ti.FileSizeMP3_128
	default:
		return 0
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/resolver.go internal/media/resolver_test.go
git commit -m "feat(media): Resolver glues SongGetData + GetURL + Cache

Exposes a TrackFetcher interface so the resolver can be tested without
importing gateway, preserving strict layering (media → gateway is
illegal). Sized to the chosen format."
```

---

### Task 8: Range-aware streaming reader

Browser `<audio>` issues Range requests. Encrypted and plaintext streams have the same byte count (Blowfish-CBC preserves block length and the partial tail is passthrough), so byte ranges map 1:1. We must align the upstream GET to a 2048-byte block, advance the decryptor's internal `blockIdx`, then trim leading bytes from the output until we hit the requested start offset.

**Files:**
- Create: `internal/media/stream.go`
- Create: `internal/media/stream_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/media/stream_test.go
package media

import (
	"bytes"
	"context"
	"crypto/cipher"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/crypto/blowfish"
)

// makeEncryptedFixture builds totalBytes worth of input where every 3rd
// 2048-byte block is encrypted with the given key, mirroring Deezer's scheme.
// Returns (plaintext, ciphertext).
func makeEncryptedFixture(t *testing.T, key [16]byte, totalBytes int) ([]byte, []byte) {
	t.Helper()
	plain := make([]byte, totalBytes)
	for i := range plain {
		plain[i] = byte(i % 251) // arbitrary deterministic pattern
	}
	cipherOut := make([]byte, totalBytes)
	bc, err := blowfish.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	for off, idx := 0, 0; off < totalBytes; off, idx = off+blockSize, idx+1 {
		end := off + blockSize
		if end > totalBytes {
			// Final partial: passthrough.
			copy(cipherOut[off:], plain[off:])
			break
		}
		if idx%encryptionStride == 0 {
			cipher.NewCBCEncrypter(bc, blowfishIV).CryptBlocks(cipherOut[off:end], plain[off:end])
		} else {
			copy(cipherOut[off:end], plain[off:end])
		}
	}
	return plain, cipherOut
}

func TestRangeStream_FullFile(t *testing.T) {
	key := KeyFromSNGID("42")
	plain, cipherBytes := makeEncryptedFixture(t, key, 6*blockSize+500)

	cdn := serveFixture(t, cipherBytes)
	rc, err := NewRangeStream(context.Background(), http.DefaultClient, RangeStreamInput{
		URL:   cdn.URL,
		Key:   key,
		Start: 0,
		End:   int64(len(plain)) - 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("decrypted bytes do not match plaintext (got len=%d want=%d)", len(got), len(plain))
	}
}

func TestRangeStream_AlignedSubrange(t *testing.T) {
	key := KeyFromSNGID("42")
	plain, cipherBytes := makeEncryptedFixture(t, key, 6*blockSize)
	cdn := serveFixture(t, cipherBytes)

	// Block-aligned range: start at block 3 (offset 6144).
	start := int64(3 * blockSize)
	end := int64(5*blockSize) - 1
	rc, err := NewRangeStream(context.Background(), http.DefaultClient, RangeStreamInput{
		URL: cdn.URL, Key: key, Start: start, End: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	want := plain[start : end+1]
	if !bytes.Equal(got, want) {
		t.Errorf("aligned subrange mismatch: got len=%d want=%d", len(got), len(want))
	}
}

func TestRangeStream_UnalignedSubrange(t *testing.T) {
	key := KeyFromSNGID("42")
	plain, cipherBytes := makeEncryptedFixture(t, key, 6*blockSize+500)
	cdn := serveFixture(t, cipherBytes)

	// Unaligned: start 100 bytes into block 4.
	start := int64(4*blockSize + 100)
	end := int64(5*blockSize + 200)
	rc, err := NewRangeStream(context.Background(), http.DefaultClient, RangeStreamInput{
		URL: cdn.URL, Key: key, Start: start, End: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	want := plain[start : end+1]
	if !bytes.Equal(got, want) {
		t.Errorf("unaligned subrange mismatch: got len=%d want=%d", len(got), len(want))
	}
}

func TestRangeStream_CDNNonPartialIsError(t *testing.T) {
	// Server returns 200 instead of 206 — our reader should reject that for a
	// requested subrange, because we cannot trust the block alignment.
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("nope")),
			Header:     make(http.Header),
		}, nil
	})
	hc := &http.Client{Transport: rt}
	_, err := NewRangeStream(context.Background(), hc, RangeStreamInput{
		URL:   "http://cdn",
		Key:   [16]byte{},
		Start: 4096,
		End:   8191,
	})
	if err == nil || !errors.Is(err, ErrCDNNoRange) {
		t.Errorf("err = %v, want ErrCDNNoRange", err)
	}
}
```

Add the test helper at the bottom of `internal/media/stream_test.go`:

```go
// serveFixture serves cipherBytes via httptest, honouring HTTP Range requests
// the way Deezer's CDN does (200 if no Range, 206 if Range).
import (
	"net/http/httptest"
	"strconv"
)

func serveFixture(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		ra := r.Header.Get("Range")
		if ra == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(200)
			_, _ = w.Write(body)
			return
		}
		// We only support "bytes=N-M".
		var start, end int64
		if _, err := fmt.Sscanf(ra, "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", 416)
			return
		}
		if end >= int64(len(body)) {
			end = int64(len(body)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(206)
		_, _ = w.Write(body[start : end+1])
	}))
	t.Cleanup(srv.Close)
	return srv
}
```

Add `"fmt"`, `"net/http/httptest"`, `"strconv"` to the file's imports.

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/media/ -run TestRangeStream -v`
Expected: FAIL — `undefined: NewRangeStream, RangeStreamInput, ErrCDNNoRange`.

- [ ] **Step 3: Implement**

Extend `internal/media/crypto.go` — expose a way to construct a `decryptReader` starting at an arbitrary block index. Add this method to `crypto.go`:

```go
// newDecryptReaderAtBlock returns a decryptReader whose stride counter starts
// at the given block index. Used by the range-stream code so the
// every-3rd-block pattern stays aligned with the underlying file's offsets.
func newDecryptReaderAtBlock(src io.Reader, key [16]byte, startBlockIdx int) *decryptReader {
	return &decryptReader{
		src:      src,
		key:      key,
		blockIdx: startBlockIdx,
	}
}
```

Create `internal/media/stream.go`:

```go
package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrCDNNoRange means the CDN ignored a Range request. The streaming proxy
// must reject this rather than serve mis-aligned bytes.
var ErrCDNNoRange = errors.New("media: CDN did not honour Range request")

// RangeStreamInput parameterises NewRangeStream.
type RangeStreamInput struct {
	URL   string
	Key   [16]byte
	Start int64 // inclusive
	End   int64 // inclusive; must be >= Start
}

// NewRangeStream issues a Range-aware GET to the encrypted CDN URL and
// returns a reader that yields plaintext bytes for the [Start, End] range.
// The reader fetches the smallest 2048-byte-aligned superset of the requested
// range, then trims the leading slack bytes.
func NewRangeStream(ctx context.Context, hc *http.Client, in RangeStreamInput) (io.ReadCloser, error) {
	if in.End < in.Start {
		return nil, fmt.Errorf("range stream: End (%d) < Start (%d)", in.End, in.Start)
	}

	// Align Start down to a block boundary; End stays inclusive.
	alignedStart := (in.Start / blockSize) * blockSize
	startBlockIdx := int(alignedStart / blockSize)
	leadingSlack := in.Start - alignedStart
	wantLen := in.End - in.Start + 1

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return nil, err
	}
	// Full-file range request: omit the upper bound so a CDN that knows the
	// total size can respond with the entire tail.
	if in.Start == 0 && in.End >= veryLarge {
		// No Range header — request the whole file.
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", alignedStart, in.End))
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cdn get: %w", err)
	}
	wantRange := req.Header.Get("Range") != ""
	if wantRange && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, ErrCDNNoRange
	}
	if !wantRange && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("cdn get: http %d", resp.StatusCode)
	}

	dec := newDecryptReaderAtBlock(resp.Body, in.Key, startBlockIdx)
	// Discard the leading slack and cap the output to wantLen bytes.
	if leadingSlack > 0 {
		if _, err := io.CopyN(io.Discard, dec, leadingSlack); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("trim leading slack: %w", err)
		}
	}
	limited := io.LimitReader(dec, wantLen)
	return readCloser{Reader: limited, closer: resp.Body}, nil
}

// veryLarge is a sentinel "no end" used by full-file callers.
const veryLarge int64 = (1 << 62)

type readCloser struct {
	io.Reader
	closer io.Closer
}

func (rc readCloser) Close() error { return rc.closer.Close() }
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/media/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/media/crypto.go internal/media/stream.go internal/media/stream_test.go
git commit -m "feat(media): range-aware streaming reader

NewRangeStream aligns Start down to a 2048-byte block boundary, seeds
the decryptor's blockIdx so the every-3rd-block pattern stays aligned
with the original file, then trims leading slack and caps the output
to the requested byte length. Rejects CDN 200 when we asked for Range
(misaligned bytes are unsafe to decrypt)."
```

---

## Phase C — Session layer

### Task 9: `session.Session` — state, queue, transport intent

**Files:**
- Create: `internal/session/track.go`
- Create: `internal/session/session.go`
- Create: `internal/session/session_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/session/session_test.go
package session

import (
	"testing"
)

func tr(id, title string) Track {
	return Track{ID: id, Title: title, DurationS: 200}
}

func TestSession_PlayTrack_ReplacesQueue(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	st := s.State()
	if st.Current == nil || st.Current.ID != "1" {
		t.Errorf("Current = %+v", st.Current)
	}
	if len(st.Queue) != 1 || st.QueuePos != 0 {
		t.Errorf("Queue = %v, Pos = %d", st.Queue, st.QueuePos)
	}
	if st.Paused {
		t.Error("Paused should be false on PlayTrack")
	}
}

func TestSession_PlayList_StartsAtIndex(t *testing.T) {
	s := New()
	tracks := []Track{tr("1", "A"), tr("2", "B"), tr("3", "C")}
	s.PlayList(tracks, 1)
	st := s.State()
	if st.Current.ID != "2" || st.QueuePos != 1 || len(st.Queue) != 3 {
		t.Errorf("state = %+v", st)
	}
}

func TestSession_PauseResume(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	s.Pause()
	if !s.State().Paused {
		t.Error("Pause not reflected")
	}
	s.Play()
	if s.State().Paused {
		t.Error("Play did not unpause")
	}
}

func TestSession_Next_Prev(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A"), tr("2", "B"), tr("3", "C")}, 0)
	next, ok := s.Next()
	if !ok || next.ID != "2" {
		t.Errorf("Next = %+v ok=%v", next, ok)
	}
	prev, ok := s.Prev()
	if !ok || prev.ID != "1" {
		t.Errorf("Prev = %+v ok=%v", prev, ok)
	}
}

func TestSession_NextAtEnd_NotOK(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A")}, 0)
	if _, ok := s.Next(); ok {
		t.Error("Next at end should be !ok")
	}
}

func TestSession_Seek_ClampsAndStores(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	s.Seek(1234)
	if pos := s.State().PositionMs; pos != 1234 {
		t.Errorf("PositionMs = %d", pos)
	}
	s.Seek(-5)
	if pos := s.State().PositionMs; pos != 0 {
		t.Errorf("Negative seek not clamped: %d", pos)
	}
}

func TestSession_SetVolume_ClampedTo01(t *testing.T) {
	s := New()
	s.SetVolume(-1)
	if s.State().Volume != 0 {
		t.Errorf("Volume = %v", s.State().Volume)
	}
	s.SetVolume(2)
	if s.State().Volume != 1 {
		t.Errorf("Volume = %v", s.State().Volume)
	}
	s.SetVolume(0.5)
	if s.State().Volume != 0.5 {
		t.Errorf("Volume = %v", s.State().Volume)
	}
}

func TestSession_UpdatePlayback_StoresPosition(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	s.UpdatePlayback(5000, false, false)
	if pos := s.State().PositionMs; pos != 5000 {
		t.Errorf("PositionMs = %d", pos)
	}
	s.UpdatePlayback(0, true, false)
	if !s.State().Paused {
		t.Error("Paused not reflected from UpdatePlayback")
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/session/ -v`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Implement**

Create `internal/session/track.go`:

```go
// Package session owns the authoritative remote-control state: current
// track, queue, paused intent, volume target, and auto-advance bookkeeping.
package session

// Track is the wire-friendly track shape exchanged with controllers and
// the player tab. CoverURL is a fully-formed image URL (the transport
// layer fills it in from cover_md5 when adapting from gateway summaries).
type Track struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	DurationS int    `json:"duration_s"`
	CoverURL  string `json:"cover_url"`
}
```

Create `internal/session/session.go`:

```go
package session

import "sync"

// State is the snapshot every controller and the player render from.
type State struct {
	Current    *Track  `json:"current,omitempty"`
	Queue      []Track `json:"queue"`
	QueuePos   int     `json:"queue_pos"`
	PositionMs int64   `json:"position_ms"`
	Paused     bool    `json:"paused"`
	Volume     float64 `json:"volume"` // 0..1
}

// Session is the goroutine-safe owner of State.
type Session struct {
	mu    sync.RWMutex
	state State
	// skipCount tracks consecutive failed loads for the auto-advance cap.
	// See advance.go.
	skipCount int
}

// New returns a Session at volume 1.0, paused, with an empty queue.
func New() *Session {
	return &Session{state: State{Volume: 1.0, Paused: true}}
}

// State returns a deep-copied snapshot safe to marshal off the lock.
func (s *Session) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.state
	if len(s.state.Queue) > 0 {
		out.Queue = append([]Track(nil), s.state.Queue...)
	}
	if s.state.Current != nil {
		t := *s.state.Current
		out.Current = &t
	}
	return out
}

// PlayTrack replaces the queue with a single track and starts unpaused.
func (s *Session) PlayTrack(t Track) {
	s.PlayList([]Track{t}, 0)
}

// PlayList replaces the queue with tracks and starts at startIdx.
func (s *Session) PlayList(tracks []Track, startIdx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(tracks) {
		startIdx = 0
	}
	s.state.Queue = append([]Track(nil), tracks...)
	s.state.QueuePos = startIdx
	if len(tracks) > 0 {
		cur := tracks[startIdx]
		s.state.Current = &cur
	} else {
		s.state.Current = nil
	}
	s.state.PositionMs = 0
	s.state.Paused = false
	s.skipCount = 0
}

// Pause sets the paused intent.
func (s *Session) Pause() { s.setPaused(true) }

// Play clears the paused intent.
func (s *Session) Play() { s.setPaused(false) }

func (s *Session) setPaused(v bool) {
	s.mu.Lock()
	s.state.Paused = v
	s.mu.Unlock()
}

// Next advances queue position by one and returns the new current track.
// Returns ok=false when already at the end.
func (s *Session) Next() (Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.QueuePos+1 >= len(s.state.Queue) {
		return Track{}, false
	}
	s.state.QueuePos++
	cur := s.state.Queue[s.state.QueuePos]
	s.state.Current = &cur
	s.state.PositionMs = 0
	s.state.Paused = false
	s.skipCount = 0
	return cur, true
}

// Prev steps queue position back by one.
func (s *Session) Prev() (Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.QueuePos == 0 {
		return Track{}, false
	}
	s.state.QueuePos--
	cur := s.state.Queue[s.state.QueuePos]
	s.state.Current = &cur
	s.state.PositionMs = 0
	s.state.Paused = false
	s.skipCount = 0
	return cur, true
}

// Seek clamps position to [0, +inf) and stores it.
func (s *Session) Seek(positionMs int64) {
	s.mu.Lock()
	if positionMs < 0 {
		positionMs = 0
	}
	s.state.PositionMs = positionMs
	s.mu.Unlock()
}

// SetVolume clamps v to [0, 1].
func (s *Session) SetVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	s.mu.Lock()
	s.state.Volume = v
	s.mu.Unlock()
}

// UpdatePlayback ingests a periodic update from the player tab.
// ended=true triggers no state change here; the caller (transport) is
// responsible for invoking Next or SkipDead.
func (s *Session) UpdatePlayback(positionMs int64, paused bool, ended bool) {
	s.mu.Lock()
	if positionMs >= 0 {
		s.state.PositionMs = positionMs
	}
	s.state.Paused = paused
	s.mu.Unlock()
	_ = ended
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/session/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/
git commit -m "feat(session): authoritative state with queue + transport intent

Goroutine-safe Session that owns Current / Queue / QueuePos /
PositionMs / Paused / Volume. State() returns a deep-copied snapshot
so callers can marshal off the lock without seeing torn state."
```

---

### Task 10: Auto-advance with 5-skip cap

`SkipDead` is called by transport when loading a track fails (e.g. `media.getUrl` returns no URL). It bumps a counter, advances the queue, and signals `exhausted` at the cap. `MarkLoaded` resets the counter on success.

**Files:**
- Create: `internal/session/advance.go`
- Create: `internal/session/advance_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/session/advance_test.go
package session

import "testing"

func TestSkipDead_Advances(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A"), tr("2", "B"), tr("3", "C")}, 0)
	next, ok, exhausted := s.SkipDead()
	if !ok || exhausted || next.ID != "2" {
		t.Errorf("SkipDead 1: next=%+v ok=%v exhausted=%v", next, ok, exhausted)
	}
}

func TestSkipDead_ExhaustsAtFifthConsecutive(t *testing.T) {
	s := New()
	tracks := []Track{tr("1", "A"), tr("2", "B"), tr("3", "C"), tr("4", "D"), tr("5", "E"), tr("6", "F"), tr("7", "G")}
	s.PlayList(tracks, 0)
	// Skip 5 times in a row.
	for i := 0; i < 4; i++ {
		_, ok, exhausted := s.SkipDead()
		if !ok || exhausted {
			t.Fatalf("skip %d: ok=%v exhausted=%v", i, ok, exhausted)
		}
	}
	_, ok, exhausted := s.SkipDead()
	if ok || !exhausted {
		t.Errorf("5th SkipDead: ok=%v exhausted=%v (want ok=false exhausted=true)", ok, exhausted)
	}
	// Exhausted pauses the session.
	if !s.State().Paused {
		t.Error("exhausted state should pause")
	}
}

func TestSkipDead_MarkLoadedResetsCounter(t *testing.T) {
	s := New()
	tracks := []Track{tr("1", "A"), tr("2", "B"), tr("3", "C"), tr("4", "D"), tr("5", "E"), tr("6", "F"), tr("7", "G")}
	s.PlayList(tracks, 0)
	for i := 0; i < 4; i++ {
		_, _, _ = s.SkipDead()
	}
	s.MarkLoaded()
	// After a successful load, the cap resets — we should be able to skip again.
	if _, ok, exhausted := s.SkipDead(); !ok || exhausted {
		t.Errorf("after MarkLoaded: ok=%v exhausted=%v", ok, exhausted)
	}
}

func TestSkipDead_NoNextTrack(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A")}, 0)
	_, ok, exhausted := s.SkipDead()
	if ok || !exhausted {
		t.Errorf("end-of-queue SkipDead: ok=%v exhausted=%v", ok, exhausted)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/session/ -run TestSkipDead -v`

- [ ] **Step 3: Implement**

Create `internal/session/advance.go`:

```go
package session

// MaxConsecutiveSkips is the upper bound on consecutive failed loads before
// the session pauses and the transport emits a queue_exhausted error.
const MaxConsecutiveSkips = 5

// SkipDead advances the queue after a failed track load. Returns:
//   - next track if there is one and the cap has not been reached
//   - ok=true when a next track is available AND skipCount < MaxConsecutiveSkips
//   - exhausted=true when the cap is reached OR the queue is empty
//
// On exhausted=true the session is paused; callers should emit
// {kind: "queue_exhausted"} over the wire.
func (s *Session) SkipDead() (Track, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.skipCount++
	if s.skipCount >= MaxConsecutiveSkips {
		s.state.Paused = true
		return Track{}, false, true
	}
	if s.state.QueuePos+1 >= len(s.state.Queue) {
		s.state.Paused = true
		return Track{}, false, true
	}
	s.state.QueuePos++
	cur := s.state.Queue[s.state.QueuePos]
	s.state.Current = &cur
	s.state.PositionMs = 0
	s.state.Paused = false
	return cur, true, false
}

// MarkLoaded resets the consecutive-skip counter. Called by transport once a
// track has been successfully resolved and started playing.
func (s *Session) MarkLoaded() {
	s.mu.Lock()
	s.skipCount = 0
	s.mu.Unlock()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/session/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/session/advance.go internal/session/advance_test.go
git commit -m "feat(session): auto-advance with 5-skip cap

SkipDead advances the queue and returns exhausted=true when 5
consecutive failed loads have occurred OR the queue ran out.
MarkLoaded resets the counter on a successful play."
```

---

## Phase D — Network helpers

### Task 11: LAN IP enumeration

`serve` and `doctor` need to pick the private LAN address(es) to print and probe. RFC1918 ranges only; skip loopback, link-local, and IPv6.

**Files:**
- Create: `internal/network/lan.go`
- Create: `internal/network/lan_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/network/lan_test.go
package network

import (
	"net"
	"testing"
)

func TestIsPrivateIPv4(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.10", true},
		{"10.0.0.1", true},
		{"172.16.5.5", true},
		{"172.31.255.255", true},
		{"172.32.0.0", false},
		{"172.15.0.0", false},
		{"8.8.8.8", false},
		{"127.0.0.1", false},
		{"169.254.1.1", false},
		{"::1", false},
		{"fe80::1", false},
	}
	for _, tc := range cases {
		got := IsPrivateIPv4(net.ParseIP(tc.ip))
		if got != tc.want {
			t.Errorf("IsPrivateIPv4(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestLANAddrs_FiltersToPrivate(t *testing.T) {
	// We can't control real interfaces; verify the synthesized filter logic
	// instead.
	in := []net.IP{
		net.ParseIP("192.168.1.10"),
		net.ParseIP("127.0.0.1"),
		net.ParseIP("fe80::1"),
		net.ParseIP("10.0.0.5"),
		net.ParseIP("8.8.8.8"),
	}
	got := filterPrivateIPv4(in)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2: %v", len(got), got)
	}
	if got[0].String() != "192.168.1.10" || got[1].String() != "10.0.0.5" {
		t.Errorf("filter = %v", got)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/network/ -v`

- [ ] **Step 3: Implement**

Create `internal/network/lan.go`:

```go
// Package network exposes LAN-discovery helpers for the serve / doctor
// commands.
package network

import (
	"net"
)

var (
	cidr10  = mustCIDR("10.0.0.0/8")
	cidr172 = mustCIDR("172.16.0.0/12")
	cidr192 = mustCIDR("192.168.0.0/16")
)

func mustCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// IsPrivateIPv4 reports whether ip is in an RFC1918 range.
func IsPrivateIPv4(ip net.IP) bool {
	if ip == nil {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return false
	}
	return cidr10.Contains(v4) || cidr172.Contains(v4) || cidr192.Contains(v4)
}

// LANAddrs returns every interface IPv4 address in an RFC1918 range, sorted
// in the order the OS returns them.
func LANAddrs() ([]net.IP, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var all []net.IP
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil {
				all = append(all, ip)
			}
		}
	}
	return filterPrivateIPv4(all), nil
}

func filterPrivateIPv4(in []net.IP) []net.IP {
	var out []net.IP
	for _, ip := range in {
		if IsPrivateIPv4(ip) {
			out = append(out, ip.To4())
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/network/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/network/
git commit -m "feat(network): LAN IPv4 enumeration (RFC1918 only)"
```

---

### Task 12: Terminal QR code rendering

Wraps `mdp/qrterminal/v3` so the cmd-layer doesn't import the lib directly. Single-function package.

**Files:**
- Create: `internal/qrterm/qrterm.go`

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/mdp/qrterminal/v3@latest`

- [ ] **Step 2: Implement**

Create `internal/qrterm/qrterm.go`:

```go
// Package qrterm renders QR codes to stdout / stderr using mdp/qrterminal.
package qrterm

import (
	"io"

	"github.com/mdp/qrterminal/v3"
)

// Render writes a QR code for content to w using compact half-block runes.
// Suitable for terminals supporting Unicode; falls back gracefully on
// monospaced ASCII fonts.
func Render(w io.Writer, content string) {
	cfg := qrterminal.Config{
		Level:      qrterminal.M,
		Writer:     w,
		HalfBlocks: true,
		BlackChar:  qrterminal.BLACK_BLACK,
		WhiteChar:  qrterminal.WHITE_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		QuietZone:  1,
	}
	qrterminal.GenerateWithConfig(content, cfg)
}
```

- [ ] **Step 3: Tidy and build**

Run:
```bash
go mod tidy
go build ./...
```
Expected: clean. The new dependency lands in `go.mod` and `go.sum`.

- [ ] **Step 4: Commit**

```bash
git add internal/qrterm/ go.mod go.sum
git commit -m "feat(qrterm): terminal QR rendering via mdp/qrterminal

Wraps the lib so cmd/* depends on internal/qrterm rather than
importing the library directly. Half-block runes keep the QR compact
on terminals with Unicode."
```

---

## Phase E — Transport layer

### Task 13: Wire message types + classified `Error`

Centralises the JSON shapes shared across HTTP and WS. No behavior here, just types.

**Files:**
- Create: `internal/transport/messages.go`

- [ ] **Step 1: Implement**

Create `internal/transport/messages.go`:

```go
// Package transport owns the HTTP + WebSocket surface that controllers and
// the player tab connect to. It depends on internal/session and
// internal/media, never the other way round.
package transport

import (
	"encoding/json"

	"github.com/niref/deezer-remote/internal/session"
)

// Error.Kind values used on the wire. Controllers and the player branch on
// these strings — never on the human-readable Message.
const (
	ErrKindAuthExpired    = "auth_expired"
	ErrKindNotAvailable   = "not_available"
	ErrKindRateLimited    = "rate_limited"
	ErrKindPlayerGone     = "player_gone"
	ErrKindRegionLocked   = "region_locked"
	ErrKindQueueExhausted = "queue_exhausted"
	ErrKindRoleTaken      = "role_taken"
	ErrKindInternal       = "internal"
)

// Role values for the initial "hello" message.
const (
	RolePlayer     = "player"
	RoleController = "controller"
)

// Cmd kinds (controller → service).
const (
	CmdPlayTrack    = "play_track"
	CmdPlayAlbum    = "play_album"
	CmdPlayPlaylist = "play_playlist"
	CmdPlay         = "play"
	CmdPause        = "pause"
	CmdNext         = "next"
	CmdPrev         = "prev"
	CmdSeek         = "seek"
	CmdSetVolume    = "set_volume"
)

// Do kinds (service → player).
const (
	DoLoad      = "load"
	DoPlay      = "play"
	DoPause     = "pause"
	DoSeek      = "seek"
	DoSetVolume = "set_volume"
)

// Message envelope. Every WS message has {"type": "..."} as its first field.
type Message struct {
	Type string `json:"type"`
}

// Hello is the first message a tab sends after connecting.
type Hello struct {
	Type string `json:"type"` // "hello"
	Role string `json:"role"` // "player" | "controller"
}

// PlaybackUpdate is sent by the player tab every ~250 ms.
type PlaybackUpdate struct {
	Type       string `json:"type"` // "playback"
	PositionMs int64  `json:"position_ms"`
	Paused     bool   `json:"paused"`
	Ended      bool   `json:"ended"`
}

// Cmd is a controller-issued command.
type Cmd struct {
	Type    string          `json:"type"` // "cmd"
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Do is a service-issued instruction to the player tab.
type Do struct {
	Type    string          `json:"type"` // "do"
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// LoadPayload is the body of a Do{Kind: DoLoad}.
type LoadPayload struct {
	TrackID  string `json:"track_id"`
	StreamURL string `json:"stream_url"` // /stream/<id>?t=<token>
}

// SeekPayload is the body of Cmd{Kind: CmdSeek} and Do{Kind: DoSeek}.
type SeekPayload struct {
	PositionMs int64 `json:"position_ms"`
}

// VolumePayload is the body of Cmd{Kind: CmdSetVolume} and Do{Kind: DoSetVolume}.
type VolumePayload struct {
	Volume float64 `json:"volume"`
}

// PlayTrackPayload is the body of Cmd{Kind: CmdPlayTrack}.
type PlayTrackPayload struct {
	TrackID string `json:"track_id"`
}

// PlayAlbumPayload is the body of Cmd{Kind: CmdPlayAlbum}.
type PlayAlbumPayload struct {
	AlbumID string `json:"album_id"`
	StartIdx int   `json:"start_idx,omitempty"`
}

// PlayPlaylistPayload is the body of Cmd{Kind: CmdPlayPlaylist}.
type PlayPlaylistPayload struct {
	PlaylistID string `json:"playlist_id"`
	StartIdx   int    `json:"start_idx,omitempty"`
}

// StateUpdate is the state snapshot fanned out to all tabs.
type StateUpdate struct {
	Type  string        `json:"type"` // "state"
	State session.State `json:"state"`
}

// ErrorMessage is the classified-kind error envelope.
type ErrorMessage struct {
	Type    string `json:"type"` // "error"
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// CoverURL builds the image-CDN URL for a track/album cover md5 hash.
// Phase 1 uses 250x250 for both phone and laptop.
func CoverURL(md5 string) string {
	if md5 == "" {
		return ""
	}
	return "https://e-cdns-images.dzcdn.net/images/cover/" + md5 + "/250x250-000000-80-0-0.jpg"
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/transport/messages.go
git commit -m "feat(transport): wire message types + classified error kinds"
```

---

### Task 14: Bearer token middleware

Accepts `Authorization: Bearer <token>` OR `?t=<token>`. Returns `401` when neither matches.

**Files:**
- Create: `internal/transport/auth.go`
- Create: `internal/transport/auth_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/transport/auth_test.go
package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireToken_AcceptsHeader(t *testing.T) {
	called := false
	h := RequireToken("SECRET")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Header.Set("Authorization", "Bearer SECRET")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called || rr.Code != 200 {
		t.Errorf("called=%v code=%d", called, rr.Code)
	}
}

func TestRequireToken_AcceptsQueryParam(t *testing.T) {
	called := false
	h := RequireToken("SECRET")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/stream/42?t=SECRET", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called || rr.Code != 200 {
		t.Errorf("called=%v code=%d", called, rr.Code)
	}
}

func TestRequireToken_Rejects(t *testing.T) {
	cases := []struct {
		name string
		mk   func() *http.Request
	}{
		{"missing", func() *http.Request { return httptest.NewRequest("GET", "/api/x", nil) }},
		{"wrong header", func() *http.Request {
			r := httptest.NewRequest("GET", "/api/x", nil)
			r.Header.Set("Authorization", "Bearer WRONG")
			return r
		}},
		{"wrong query", func() *http.Request {
			return httptest.NewRequest("GET", "/stream/42?t=WRONG", nil)
		}},
		{"basic auth", func() *http.Request {
			r := httptest.NewRequest("GET", "/api/x", nil)
			r.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
			return r
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := RequireToken("SECRET")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("inner handler must not be called")
			}))
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, tc.mk())
			if rr.Code != 401 {
				t.Errorf("code = %d, want 401", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "unauthorized") {
				t.Errorf("body = %q", rr.Body.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/transport/ -run TestRequireToken -v`

- [ ] **Step 3: Implement**

Create `internal/transport/auth.go`:

```go
package transport

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireToken returns middleware that admits requests carrying either
// `Authorization: Bearer <expected>` or `?t=<expected>`. Mismatches return
// HTTP 401 with a plain-text body. Constant-time compare avoids leaking
// token bytes via timing.
func RequireToken(expected string) func(http.Handler) http.Handler {
	want := []byte(expected)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if matchHeader(r, want) || matchQuery(r, want) {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

func matchHeader(r *http.Request, want []byte) bool {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.TrimPrefix(h, "Bearer ")), want) == 1
}

func matchQuery(r *http.Request, want []byte) bool {
	got := r.URL.Query().Get("t")
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), want) == 1
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/transport/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/auth.go internal/transport/auth_test.go
git commit -m "feat(transport): bearer token middleware

Accepts Authorization: Bearer or ?t= query param (the latter is the
only option for <audio src>). Constant-time compare to avoid timing
leaks against a 32-byte secret."
```

---

### Task 15: `/api/search`, `/api/track`, `/api/album`, `/api/playlist`

JSON handlers that wrap the gateway methods and convert `CoverMD5` → full image URL.

**Files:**
- Create: `internal/transport/api.go`
- Create: `internal/transport/api_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/transport/api_test.go
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/niref/deezer-remote/internal/gateway"
)

// fakeGW is a tiny double for the gateway methods APIHandlers needs.
type fakeGW struct {
	search   func(ctx context.Context, q string, n int) (*gateway.SearchResult, error)
	track    func(ctx context.Context, id string) (*gateway.TrackData, error)
	album    func(ctx context.Context, id string) (*gateway.Album, error)
	playlist func(ctx context.Context, id string) (*gateway.Playlist, error)
}

func (f fakeGW) Search(ctx context.Context, q string, n int) (*gateway.SearchResult, error) {
	return f.search(ctx, q, n)
}
func (f fakeGW) SongGetData(ctx context.Context, id string) (*gateway.TrackData, error) {
	return f.track(ctx, id)
}
func (f fakeGW) Album(ctx context.Context, id string) (*gateway.Album, error) {
	return f.album(ctx, id)
}
func (f fakeGW) Playlist(ctx context.Context, id string) (*gateway.Playlist, error) {
	return f.playlist(ctx, id)
}

func TestAPI_Search_PassesQueryAndShapesResponse(t *testing.T) {
	var seenQuery string
	gw := fakeGW{
		search: func(ctx context.Context, q string, n int) (*gateway.SearchResult, error) {
			seenQuery = q
			return &gateway.SearchResult{
				Tracks: []gateway.TrackSummary{{ID: "1", Title: "T", Artist: "A", Album: "Z", CoverMD5: "md5x", DurationS: 200}},
			}, nil
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/search?q=daft", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code = %d body = %s", rr.Code, rr.Body)
	}
	if seenQuery != "daft" {
		t.Errorf("seenQuery = %q", seenQuery)
	}
	var body struct {
		Tracks []struct {
			ID       string `json:"id"`
			CoverURL string `json:"cover_url"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tracks) != 1 || body.Tracks[0].ID != "1" {
		t.Errorf("body = %+v", body)
	}
	if !strings.Contains(body.Tracks[0].CoverURL, "md5x") {
		t.Errorf("CoverURL = %q (expected to contain md5x)", body.Tracks[0].CoverURL)
	}
}

func TestAPI_Search_400OnMissingQ(t *testing.T) {
	mux := http.NewServeMux()
	NewAPIHandlers(fakeGW{}).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/search", nil))
	if rr.Code != 400 {
		t.Errorf("code = %d", rr.Code)
	}
}

func TestAPI_Album(t *testing.T) {
	gw := fakeGW{
		album: func(ctx context.Context, id string) (*gateway.Album, error) {
			if id != "10" {
				t.Errorf("id = %q", id)
			}
			return &gateway.Album{
				Header: gateway.AlbumSummary{ID: "10", Title: "RAM", Artist: "Daft Punk", CoverMD5: "pic", TrackCount: 2},
				Tracks: []gateway.TrackSummary{
					{ID: "1", Title: "A", Artist: "Daft Punk", Album: "RAM", CoverMD5: "pic", DurationS: 200},
					{ID: "2", Title: "B", Artist: "Daft Punk", Album: "RAM", CoverMD5: "pic", DurationS: 220},
				},
			}, nil
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/album/10", nil))
	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body)
	}
}

func TestAPI_Track_404OnNotFound(t *testing.T) {
	gw := fakeGW{
		track: func(ctx context.Context, id string) (*gateway.TrackData, error) {
			return nil, gateway.ErrNotFound
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/track/999", nil))
	if rr.Code != 404 {
		t.Errorf("code = %d", rr.Code)
	}
}

func TestAPI_AuthErrors_Are401(t *testing.T) {
	gw := fakeGW{
		search: func(ctx context.Context, q string, n int) (*gateway.SearchResult, error) {
			return nil, errors.Join(gateway.ErrAuthFailed)
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/search?q=daft", nil))
	if rr.Code != 401 {
		t.Errorf("code = %d", rr.Code)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/transport/ -run TestAPI -v`

- [ ] **Step 3: Implement**

Create `internal/transport/api.go`:

```go
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/niref/deezer-remote/internal/gateway"
)

// GatewayAPI is the subset of gateway.Client the HTTP handlers need.
// Defined as an interface so api_test.go can substitute a fake.
type GatewayAPI interface {
	Search(ctx context.Context, query string, limit int) (*gateway.SearchResult, error)
	SongGetData(ctx context.Context, trackID string) (*gateway.TrackData, error)
	Album(ctx context.Context, albumID string) (*gateway.Album, error)
	Playlist(ctx context.Context, playlistID string) (*gateway.Playlist, error)
}

// APIHandlers wires /api/* endpoints over a GatewayAPI.
type APIHandlers struct {
	gw GatewayAPI
}

// NewAPIHandlers builds a set of handlers bound to gw.
func NewAPIHandlers(gw GatewayAPI) *APIHandlers {
	return &APIHandlers{gw: gw}
}

// Register attaches handlers to mux. Caller is responsible for wrapping with
// RequireToken middleware before mounting on the public listener.
func (h *APIHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/search", h.search)
	mux.HandleFunc("GET /api/track/{id}", h.track)
	mux.HandleFunc("GET /api/album/{id}", h.album)
	mux.HandleFunc("GET /api/playlist/{id}", h.playlist)
}

func (h *APIHandlers) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "missing q", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	res, err := h.gw.Search(r.Context(), q, limit)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, searchToAPI(res))
}

func (h *APIHandlers) track(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	td, err := h.gw.SongGetData(r.Context(), id)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, trackDataToAPI(td))
}

func (h *APIHandlers) album(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := h.gw.Album(r.Context(), id)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, albumToAPI(a))
}

func (h *APIHandlers) playlist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.gw.Playlist(r.Context(), id)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, playlistToAPI(p))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeGatewayError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gateway.ErrAuthFailed):
		http.Error(w, "auth_expired", http.StatusUnauthorized)
	case errors.Is(err, gateway.ErrNotFound):
		http.Error(w, "not_found", http.StatusNotFound)
	case errors.Is(err, gateway.ErrRateLimited):
		http.Error(w, "rate_limited", http.StatusTooManyRequests)
	default:
		http.Error(w, "internal: "+err.Error(), http.StatusInternalServerError)
	}
}

// Output shapes (subset of gateway types with CoverURL filled in).

type apiTrack struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	DurationS int    `json:"duration_s"`
	CoverURL  string `json:"cover_url"`
}

type apiAlbumHeader struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	CoverURL   string `json:"cover_url"`
	TrackCount int    `json:"track_count"`
}

type apiPlaylistHeader struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Owner      string `json:"owner"`
	CoverURL   string `json:"cover_url"`
	TrackCount int    `json:"track_count"`
}

type apiSearchResult struct {
	Tracks    []apiTrack          `json:"tracks"`
	Albums    []apiAlbumHeader    `json:"albums"`
	Playlists []apiPlaylistHeader `json:"playlists"`
}

type apiAlbum struct {
	Header apiAlbumHeader `json:"header"`
	Tracks []apiTrack     `json:"tracks"`
}

type apiPlaylist struct {
	Header apiPlaylistHeader `json:"header"`
	Tracks []apiTrack        `json:"tracks"`
}

func summaryToAPI(t gateway.TrackSummary) apiTrack {
	return apiTrack{
		ID: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album,
		DurationS: t.DurationS, CoverURL: CoverURL(t.CoverMD5),
	}
}

func albumHeaderToAPI(h gateway.AlbumSummary) apiAlbumHeader {
	return apiAlbumHeader{
		ID: h.ID, Title: h.Title, Artist: h.Artist,
		CoverURL: CoverURL(h.CoverMD5), TrackCount: h.TrackCount,
	}
}

func playlistHeaderToAPI(h gateway.PlaylistSummary) apiPlaylistHeader {
	return apiPlaylistHeader{
		ID: h.ID, Title: h.Title, Owner: h.Owner,
		CoverURL: CoverURL(h.CoverMD5), TrackCount: h.TrackCount,
	}
}

func searchToAPI(r *gateway.SearchResult) apiSearchResult {
	out := apiSearchResult{
		Tracks:    make([]apiTrack, 0, len(r.Tracks)),
		Albums:    make([]apiAlbumHeader, 0, len(r.Albums)),
		Playlists: make([]apiPlaylistHeader, 0, len(r.Playlists)),
	}
	for _, t := range r.Tracks {
		out.Tracks = append(out.Tracks, summaryToAPI(t))
	}
	for _, a := range r.Albums {
		out.Albums = append(out.Albums, albumHeaderToAPI(a))
	}
	for _, p := range r.Playlists {
		out.Playlists = append(out.Playlists, playlistHeaderToAPI(p))
	}
	return out
}

func albumToAPI(a *gateway.Album) apiAlbum {
	out := apiAlbum{Header: albumHeaderToAPI(a.Header), Tracks: make([]apiTrack, 0, len(a.Tracks))}
	for _, t := range a.Tracks {
		out.Tracks = append(out.Tracks, summaryToAPI(t))
	}
	return out
}

func playlistToAPI(p *gateway.Playlist) apiPlaylist {
	out := apiPlaylist{Header: playlistHeaderToAPI(p.Header), Tracks: make([]apiTrack, 0, len(p.Tracks))}
	for _, t := range p.Tracks {
		out.Tracks = append(out.Tracks, summaryToAPI(t))
	}
	return out
}

func trackDataToAPI(td *gateway.TrackData) apiTrack {
	return apiTrack{
		ID: td.SngID, Title: td.Title, Artist: td.Artist, Album: td.Album,
		DurationS: td.DurationS, CoverURL: CoverURL(td.CoverMD5),
	}
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/transport/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/api.go internal/transport/api_test.go
git commit -m "feat(transport): /api/search, /api/track/{id}, /api/album/{id}, /api/playlist/{id}

JSON handlers over a GatewayAPI interface so tests can fake the
gateway. CoverMD5 hashes are expanded to full image-CDN URLs.
gateway errors are mapped to 401 / 404 / 429 / 500."
```

---

### Task 16: `/stream/<id>` handler with Range support

Glues `media.Resolver` + `media.NewRangeStream` to HTTP. Honours browser Range requests for seek.

**Files:**
- Create: `internal/transport/stream.go`
- Create: `internal/transport/stream_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/transport/stream_test.go
package transport

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/niref/deezer-remote/internal/media"
	"golang.org/x/crypto/blowfish"
)

const testBlockSize = 2048
const testEncStride = 3

func buildEncryptedFile(t *testing.T, key [16]byte, total int) ([]byte, []byte) {
	t.Helper()
	plain := make([]byte, total)
	for i := range plain {
		plain[i] = byte(i % 251)
	}
	enc := make([]byte, total)
	bc, err := blowfish.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	for off, idx := 0, 0; off < total; off, idx = off+testBlockSize, idx+1 {
		end := off + testBlockSize
		if end > total {
			copy(enc[off:], plain[off:])
			break
		}
		if idx%testEncStride == 0 {
			cipher.NewCBCEncrypter(bc, iv).CryptBlocks(enc[off:end], plain[off:end])
		} else {
			copy(enc[off:end], plain[off:end])
		}
	}
	return plain, enc
}

func serveBytes(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ra := r.Header.Get("Range")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		if ra == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(200)
			_, _ = w.Write(body)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(ra, "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", 416)
			return
		}
		if end >= int64(len(body)) {
			end = int64(len(body)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(206)
		_, _ = w.Write(body[start : end+1])
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeResolver lets stream tests inject a pre-fabricated ResolvedTrack.
type fakeResolver struct {
	fn func(ctx context.Context, trackID string) (media.ResolvedTrack, error)
}

func (f fakeResolver) Resolve(ctx context.Context, id string) (media.ResolvedTrack, error) {
	return f.fn(ctx, id)
}

func TestStream_ServesFullFile(t *testing.T) {
	key := media.KeyFromSNGID("42")
	total := 6*testBlockSize + 500
	plain, enc := buildEncryptedFile(t, key, total)
	cdn := serveBytes(t, enc)

	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{
			TrackID: "42", SngID: "42", URL: cdn.URL,
			Format: media.FormatMP3_320, Size: int64(total), ExpiryAt: time.Now().Add(time.Hour),
		}, nil
	}}

	mux := http.NewServeMux()
	NewStreamHandler(res, http.DefaultClient).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/stream/42", nil))

	if rr.Code != 200 {
		t.Fatalf("code = %d body = %s", rr.Code, rr.Body)
	}
	if rr.Header().Get("Content-Length") != strconv.Itoa(total) {
		t.Errorf("Content-Length = %q want %d", rr.Header().Get("Content-Length"), total)
	}
	if rr.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges = %q", rr.Header().Get("Accept-Ranges"))
	}
	if rr.Header().Get("Content-Type") != "audio/mpeg" {
		t.Errorf("Content-Type = %q", rr.Header().Get("Content-Type"))
	}
	got, _ := io.ReadAll(rr.Body)
	if len(got) != len(plain) {
		t.Errorf("len(got)=%d, len(plain)=%d", len(got), len(plain))
	}
}

func TestStream_ServesPartialOnRange(t *testing.T) {
	key := media.KeyFromSNGID("42")
	total := 6*testBlockSize + 100
	plain, enc := buildEncryptedFile(t, key, total)
	cdn := serveBytes(t, enc)

	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{
			TrackID: "42", SngID: "42", URL: cdn.URL,
			Format: media.FormatMP3_320, Size: int64(total), ExpiryAt: time.Now().Add(time.Hour),
		}, nil
	}}

	mux := http.NewServeMux()
	NewStreamHandler(res, http.DefaultClient).Register(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream/42", nil)
	req.Header.Set("Range", "bytes=4096-8191")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("code = %d body = %s", rr.Code, rr.Body)
	}
	if rr.Header().Get("Content-Length") != "4096" {
		t.Errorf("Content-Length = %q", rr.Header().Get("Content-Length"))
	}
	if rr.Header().Get("Content-Range") != fmt.Sprintf("bytes 4096-8191/%d", total) {
		t.Errorf("Content-Range = %q", rr.Header().Get("Content-Range"))
	}
	got, _ := io.ReadAll(rr.Body)
	if string(got) != string(plain[4096:8192]) {
		t.Error("decrypted bytes mismatch")
	}
}

func TestStream_NotAvailable_Is404(t *testing.T) {
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{}, errors.New("get_url: track unavailable")
	}}
	mux := http.NewServeMux()
	NewStreamHandler(res, http.DefaultClient).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/stream/42", nil))
	if rr.Code != 404 {
		t.Errorf("code = %d", rr.Code)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/transport/ -run TestStream -v`

- [ ] **Step 3: Implement**

Create `internal/transport/stream.go`:

```go
package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/niref/deezer-remote/internal/media"
)

// Resolver is the subset of media.Resolver the handler needs.
type Resolver interface {
	Resolve(ctx context.Context, trackID string) (media.ResolvedTrack, error)
}

// StreamHandler serves decrypted MP3 bytes over /stream/{id}.
type StreamHandler struct {
	res   Resolver
	httpc *http.Client
}

// NewStreamHandler wires a handler. httpc is the CDN-facing client; pass
// http.DefaultClient unless tests need otherwise.
func NewStreamHandler(res Resolver, httpc *http.Client) *StreamHandler {
	return &StreamHandler{res: res, httpc: httpc}
}

// Register attaches the handler at /stream/{id}. Caller wraps with RequireToken.
func (h *StreamHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /stream/{id}", h.serve)
}

func (h *StreamHandler) serve(w http.ResponseWriter, r *http.Request) {
	trackID := r.PathValue("id")
	rt, err := h.res.Resolve(r.Context(), trackID)
	if err != nil {
		http.Error(w, "not_available: "+err.Error(), http.StatusNotFound)
		return
	}

	start, end, partial, ok := parseRangeHeader(r.Header.Get("Range"), rt.Size)
	if !ok {
		http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	key := media.KeyFromSNGID(rt.SngID)
	body, err := media.NewRangeStream(r.Context(), h.httpc, media.RangeStreamInput{
		URL:   rt.URL,
		Key:   key,
		Start: start,
		End:   end,
	})
	if err != nil {
		http.Error(w, "stream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, rt.Size))
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = io.Copy(w, body)
}

// parseRangeHeader extracts a single byte range. Supports "bytes=N-M",
// "bytes=N-" (open-ended), and missing / empty Range (full file).
// Returns ok=false on syntactically invalid input.
func parseRangeHeader(h string, total int64) (start, end int64, partial, ok bool) {
	if h == "" {
		return 0, total - 1, false, true
	}
	if !strings.HasPrefix(h, "bytes=") {
		return 0, 0, false, false
	}
	spec := strings.TrimPrefix(h, "bytes=")
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false, false
	}
	s, e := spec[:dash], spec[dash+1:]
	var err error
	if s == "" {
		// Suffix range "bytes=-N": last N bytes.
		var n int64
		n, err = strconv.ParseInt(e, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false, false
		}
		start = total - n
		if start < 0 {
			start = 0
		}
		end = total - 1
	} else {
		start, err = strconv.ParseInt(s, 10, 64)
		if err != nil || start < 0 {
			return 0, 0, false, false
		}
		if e == "" {
			end = total - 1
		} else {
			end, err = strconv.ParseInt(e, 10, 64)
			if err != nil || end < start {
				return 0, 0, false, false
			}
		}
	}
	if end >= total {
		end = total - 1
	}
	if start >= total {
		return 0, 0, false, false
	}
	return start, end, true, true
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/transport/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/stream.go internal/transport/stream_test.go
git commit -m "feat(transport): /stream/{id} with Range, decrypted on the fly

Glues media.Resolver + media.NewRangeStream into an HTTP handler.
Honours bytes=N-M, bytes=N-, and bytes=-N range syntaxes; returns 206
with Content-Range on partial, 200 on full file."
```

---

### Task 17: WebSocket hub — accept, role negotiation, fan-out, heartbeat

The hub owns all live WS connections. One player slot (rejects second player), many controllers. Ping every 10 s; drop after 30 s without pong.

**Files:**
- Create: `internal/transport/hub.go`
- Create: `internal/transport/hub_test.go`

- [ ] **Step 1: Add the dependency**

Run: `go get github.com/coder/websocket@latest`

- [ ] **Step 2: Write the failing tests**

```go
// internal/transport/hub_test.go
package transport

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sendJSON(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, c *websocket.Conn, out any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal %q: %v", b, err)
	}
}

func TestHub_HelloPlayer_GetsStateSnapshot(t *testing.T) {
	hub := NewHub(nopRouter{})
	srv := httptest.NewServer(hub)
	defer srv.Close()
	c := dial(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	sendJSON(t, c, Hello{Type: "hello", Role: RolePlayer})
	var got map[string]any
	readJSON(t, c, &got)
	if got["type"] != "state" {
		t.Errorf("first msg = %v", got)
	}
}

func TestHub_SecondPlayer_GetsRoleTakenAndDisconnected(t *testing.T) {
	hub := NewHub(nopRouter{})
	srv := httptest.NewServer(hub)
	defer srv.Close()

	first := dial(t, srv)
	defer first.Close(websocket.StatusNormalClosure, "")
	sendJSON(t, first, Hello{Type: "hello", Role: RolePlayer})
	var ignored map[string]any
	readJSON(t, first, &ignored) // consume the state

	second := dial(t, srv)
	sendJSON(t, second, Hello{Type: "hello", Role: RolePlayer})
	var got map[string]any
	readJSON(t, second, &got)
	if got["type"] != "error" || got["kind"] != ErrKindRoleTaken {
		t.Errorf("second player got %v", got)
	}
}

func TestHub_BroadcastReachesAllControllers(t *testing.T) {
	hub := NewHub(nopRouter{})
	srv := httptest.NewServer(hub)
	defer srv.Close()

	a := dial(t, srv); defer a.Close(websocket.StatusNormalClosure, "")
	b := dial(t, srv); defer b.Close(websocket.StatusNormalClosure, "")

	sendJSON(t, a, Hello{Type: "hello", Role: RoleController})
	readJSON(t, a, &map[string]any{}) // consume welcome state
	sendJSON(t, b, Hello{Type: "hello", Role: RoleController})
	readJSON(t, b, &map[string]any{})

	hub.Broadcast(ErrorMessage{Type: "error", Kind: "internal", Message: "test"})
	var ga, gb map[string]any
	readJSON(t, a, &ga)
	readJSON(t, b, &gb)
	if ga["kind"] != "internal" || gb["kind"] != "internal" {
		t.Errorf("ga=%v gb=%v", ga, gb)
	}
}

// nopRouter is a Router that ignores everything.
type nopRouter struct{}

func (nopRouter) OnCmd(Cmd)               {}
func (nopRouter) OnPlayback(PlaybackUpdate) {}
func (nopRouter) State() any              { return map[string]any{"type": "state"} }
```

- [ ] **Step 3: Run, expect FAIL**

Run: `go test ./internal/transport/ -run TestHub -v`

- [ ] **Step 4: Implement**

Create `internal/transport/hub.go`:

```go
package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	heartbeatInterval = 10 * time.Second
	heartbeatTimeout  = 30 * time.Second
	writeTimeout      = 5 * time.Second
	readBufBytes      = 64 << 10
)

// Router is what the hub talks to when a tab sends a cmd or playback update.
// Implemented by the command router (Task 18). Decoupled so hub tests don't
// need a real session.
type Router interface {
	OnCmd(c Cmd)
	OnPlayback(p PlaybackUpdate)
	State() any // returns a value JSON-encoded as a "state" snapshot
}

// Hub owns live WebSocket connections. Implements http.Handler.
type Hub struct {
	r       Router
	mu      sync.Mutex
	player  *conn        // at most one
	ctrls   map[*conn]struct{}
	closed  bool
}

// NewHub builds an empty hub bound to router r.
func NewHub(r Router) *Hub {
	return &Hub{r: r, ctrls: map[*conn]struct{}{}}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // same-origin LAN; no Origin to enforce
	})
	if err != nil {
		return
	}
	c.SetReadLimit(readBufBytes)
	co := &conn{ws: c, hub: h, role: "", out: make(chan []byte, 32)}
	go co.writer()
	co.run(r.Context())
}

// Broadcast queues v on every controller (and the player). Drops on full
// buffer rather than blocking.
func (h *Hub) Broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	targets := make([]*conn, 0, len(h.ctrls)+1)
	if h.player != nil {
		targets = append(targets, h.player)
	}
	for c := range h.ctrls {
		targets = append(targets, c)
	}
	h.mu.Unlock()
	for _, c := range targets {
		select {
		case c.out <- b:
		default:
			// Drop; slow consumer.
		}
	}
}

// SendToPlayer queues v on the player conn, if any.
func (h *Hub) SendToPlayer(v any) bool {
	h.mu.Lock()
	p := h.player
	h.mu.Unlock()
	if p == nil {
		return false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	select {
	case p.out <- b:
		return true
	default:
		return false
	}
}

// HasPlayer reports whether a player tab is currently connected.
func (h *Hub) HasPlayer() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.player != nil
}

func (h *Hub) registerPlayer(c *conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.player != nil {
		return false
	}
	h.player = c
	return true
}

func (h *Hub) registerController(c *conn) {
	h.mu.Lock()
	h.ctrls[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) drop(c *conn) {
	h.mu.Lock()
	if h.player == c {
		h.player = nil
	}
	delete(h.ctrls, c)
	h.mu.Unlock()
}

// conn is one WS connection.
type conn struct {
	ws   *websocket.Conn
	hub  *Hub
	role string
	out  chan []byte
}

func (c *conn) writer() {
	for b := range c.out {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err := c.ws.Write(ctx, websocket.MessageText, b)
		cancel()
		if err != nil {
			return
		}
	}
}

func (c *conn) run(ctx context.Context) {
	defer func() {
		c.hub.drop(c)
		close(c.out)
		_ = c.ws.Close(websocket.StatusNormalClosure, "bye")
	}()

	// Heartbeat: ping every heartbeatInterval; close on timeout.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go c.heartbeat(ctx)

	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var env Message
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case "hello":
			var h Hello
			if err := json.Unmarshal(data, &h); err != nil {
				continue
			}
			c.role = h.Role
			c.onHello()
		case "cmd":
			var cm Cmd
			if err := json.Unmarshal(data, &cm); err != nil {
				continue
			}
			c.hub.r.OnCmd(cm)
		case "playback":
			var p PlaybackUpdate
			if err := json.Unmarshal(data, &p); err != nil {
				continue
			}
			c.hub.r.OnPlayback(p)
		}
	}
}

func (c *conn) onHello() {
	switch c.role {
	case RolePlayer:
		if !c.hub.registerPlayer(c) {
			b, _ := json.Marshal(ErrorMessage{Type: "error", Kind: ErrKindRoleTaken, Message: "another player is connected"})
			c.out <- b
			// Close shortly after so the dialer sees the error.
			go func() {
				time.Sleep(50 * time.Millisecond)
				_ = c.ws.Close(websocket.StatusPolicyViolation, "role_taken")
			}()
			return
		}
	case RoleController:
		c.hub.registerController(c)
	default:
		_ = c.ws.Close(websocket.StatusUnsupportedData, "bad role")
		return
	}
	b, _ := json.Marshal(c.hub.r.State())
	c.out <- b
}

func (c *conn) heartbeat(ctx context.Context) {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
			err := c.ws.Ping(pctx)
			cancel()
			if err != nil {
				_ = c.ws.Close(websocket.StatusGoingAway, "heartbeat")
				return
			}
		}
	}
}
```

- [ ] **Step 5: Tidy and test**

Run:
```bash
go mod tidy
go test ./internal/transport/ -v
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/transport/hub.go internal/transport/hub_test.go go.mod go.sum
git commit -m "feat(transport): WebSocket hub with role negotiation + heartbeat

One player slot (returns role_taken on second). Many controllers.
Per-conn outbound buffer of 32 messages; drop-on-full rather than
block the hub. Ping every 10s, 30s timeout."
```

---

### Task 18: Command router — wires session + media + hub

When a controller sends a cmd, the router applies it to the session and, if a track changed, instructs the player via `do {kind: "load"}`. When the player reports `ended`, the router calls `Next()` / `SkipDead()` and loops.

**Files:**
- Create: `internal/transport/router.go`
- Create: `internal/transport/router_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/transport/router_test.go
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/session"
)

// fakeHub captures Broadcast / SendToPlayer for inspection.
type fakeHub struct {
	mu        sync.Mutex
	bc        []any
	toPlayer  []any
	hasPlayer bool
}

func (h *fakeHub) Broadcast(v any) {
	h.mu.Lock(); h.bc = append(h.bc, v); h.mu.Unlock()
}
func (h *fakeHub) SendToPlayer(v any) bool {
	h.mu.Lock(); h.toPlayer = append(h.toPlayer, v); h.mu.Unlock()
	return h.hasPlayer
}
func (h *fakeHub) HasPlayer() bool { return h.hasPlayer }

func TestRouter_PlayTrack_LoadsAndBroadcasts(t *testing.T) {
	s := session.New()
	gw := fakeGW{
		track: func(ctx context.Context, id string) (*gateway.TrackData, error) {
			return &gateway.TrackData{SngID: id, Title: "T", Artist: "A", Album: "Z", CoverMD5: "p", DurationS: 200}, nil
		},
	}
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{TrackID: id, SngID: id, URL: "https://cdn/x", Format: media.FormatMP3_320, Size: 100, ExpiryAt: time.Now().Add(time.Hour)}, nil
	}}
	hub := &fakeHub{hasPlayer: true}
	r := NewRouter(s, gw, res, hub, "TOKEN")

	payload, _ := json.Marshal(PlayTrackPayload{TrackID: "42"})
	r.OnCmd(Cmd{Type: "cmd", Kind: CmdPlayTrack, Payload: payload})

	// Wait briefly for async resolve (router may run resolve in a goroutine; if
	// it's synchronous this still passes).
	time.Sleep(50 * time.Millisecond)

	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.toPlayer) == 0 {
		t.Fatal("expected at least one Do message to player")
	}
	loadFound := false
	for _, v := range hub.toPlayer {
		d, _ := v.(Do)
		if d.Kind == DoLoad {
			loadFound = true
		}
	}
	if !loadFound {
		t.Error("no do:load sent to player")
	}
	if s.State().Current == nil || s.State().Current.ID != "42" {
		t.Errorf("session.Current = %+v", s.State().Current)
	}
}

func TestRouter_OnPlaybackEnded_Advances(t *testing.T) {
	s := session.New()
	s.PlayList([]session.Track{{ID: "1"}, {ID: "2"}}, 0)
	gw := fakeGW{}
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{TrackID: id, SngID: id, URL: "u"}, nil
	}}
	hub := &fakeHub{hasPlayer: true}
	r := NewRouter(s, gw, res, hub, "TOKEN")
	r.OnPlayback(PlaybackUpdate{Type: "playback", Ended: true})
	time.Sleep(50 * time.Millisecond)
	if s.State().Current == nil || s.State().Current.ID != "2" {
		t.Errorf("session.Current after ended = %+v", s.State().Current)
	}
}

func TestRouter_OnPlaybackEnded_NotAvailable_Skips(t *testing.T) {
	s := session.New()
	s.PlayList([]session.Track{{ID: "1"}, {ID: "2"}, {ID: "3"}}, 0)
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		if id == "2" {
			return media.ResolvedTrack{}, errors.New("not available")
		}
		return media.ResolvedTrack{TrackID: id, SngID: id, URL: "u"}, nil
	}}
	hub := &fakeHub{hasPlayer: true}
	r := NewRouter(s, fakeGW{}, res, hub, "TOKEN")

	r.OnPlayback(PlaybackUpdate{Type: "playback", Ended: true})
	time.Sleep(100 * time.Millisecond)

	// Should have skipped 2 and landed on 3.
	if s.State().Current == nil || s.State().Current.ID != "3" {
		t.Errorf("session.Current = %+v", s.State().Current)
	}
	// And an error broadcast about 'not_available' should have fired.
	hub.mu.Lock()
	defer hub.mu.Unlock()
	sawNotAvail := false
	for _, v := range hub.bc {
		e, ok := v.(ErrorMessage)
		if ok && e.Kind == ErrKindNotAvailable {
			sawNotAvail = true
		}
	}
	if !sawNotAvail {
		t.Error("expected a broadcasted not_available error")
	}
}

func TestRouter_State_ReturnsStateMessage(t *testing.T) {
	s := session.New()
	r := NewRouter(s, fakeGW{}, fakeResolver{}, &fakeHub{}, "TOKEN")
	v := r.State()
	su, ok := v.(StateUpdate)
	if !ok {
		t.Fatalf("State() = %T, want StateUpdate", v)
	}
	if su.Type != "state" {
		t.Errorf("Type = %q", su.Type)
	}
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/transport/ -run TestRouter -v`

- [ ] **Step 3: Implement**

Create `internal/transport/router.go`:

```go
package transport

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/session"
)

// HubSink is the subset of Hub the router talks to. Defined so router_test
// can use a fakeHub.
type HubSink interface {
	Broadcast(v any)
	SendToPlayer(v any) bool
	HasPlayer() bool
}

// CmdRouter applies controller cmds to the session, instructs the player,
// and broadcasts state changes.
type CmdRouter struct {
	sess  *session.Session
	gw    GatewayAPI
	res   Resolver
	hub   HubSink
	token string
}

// NewRouter wires a router. token is the bearer token; it's appended to
// /stream/<id> URLs so the player's <audio> tag can fetch them.
func NewRouter(sess *session.Session, gw GatewayAPI, res Resolver, hub HubSink, token string) *CmdRouter {
	return &CmdRouter{sess: sess, gw: gw, res: res, hub: hub, token: token}
}

// State returns the current state snapshot wrapped in a StateUpdate.
func (r *CmdRouter) State() any {
	return StateUpdate{Type: "state", State: r.sess.State()}
}

// OnCmd applies a single controller command.
func (r *CmdRouter) OnCmd(c Cmd) {
	switch c.Kind {
	case CmdPlayTrack:
		var p PlayTrackPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		go r.playTrack(p.TrackID)
	case CmdPlayAlbum:
		var p PlayAlbumPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		go r.playAlbum(p.AlbumID, p.StartIdx)
	case CmdPlayPlaylist:
		var p PlayPlaylistPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		go r.playPlaylist(p.PlaylistID, p.StartIdx)
	case CmdPlay:
		r.sess.Play()
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoPlay})
		r.broadcastState()
	case CmdPause:
		r.sess.Pause()
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoPause})
		r.broadcastState()
	case CmdNext:
		go r.advance(false)
	case CmdPrev:
		if t, ok := r.sess.Prev(); ok {
			r.loadAndBroadcast(t.ID)
		}
	case CmdSeek:
		var p SeekPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		r.sess.Seek(p.PositionMs)
		pb, _ := json.Marshal(p)
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoSeek, Payload: pb})
		r.broadcastState()
	case CmdSetVolume:
		var p VolumePayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		r.sess.SetVolume(p.Volume)
		pb, _ := json.Marshal(p)
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoSetVolume, Payload: pb})
		r.broadcastState()
	}
}

// OnPlayback is called for every {type:"playback"} message from the player.
func (r *CmdRouter) OnPlayback(p PlaybackUpdate) {
	r.sess.UpdatePlayback(p.PositionMs, p.Paused, p.Ended)
	if p.Ended {
		go r.advance(false)
	} else {
		r.broadcastState()
	}
}

// playTrack resolves a single track and starts it.
func (r *CmdRouter) playTrack(trackID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	td, err := r.gw.SongGetData(ctx, trackID)
	if err != nil {
		r.broadcastError(ErrKindNotAvailable, err.Error())
		return
	}
	r.sess.PlayTrack(toSessionTrack(td))
	r.loadAndBroadcast(trackID)
}

func (r *CmdRouter) playAlbum(albumID string, startIdx int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := r.gw.Album(ctx, albumID)
	if err != nil {
		r.broadcastError(ErrKindNotAvailable, err.Error())
		return
	}
	r.sess.PlayList(summariesToSessionTracks(a.Tracks), startIdx)
	if cur := r.sess.State().Current; cur != nil {
		r.loadAndBroadcast(cur.ID)
	}
}

func (r *CmdRouter) playPlaylist(playlistID string, startIdx int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := r.gw.Playlist(ctx, playlistID)
	if err != nil {
		r.broadcastError(ErrKindNotAvailable, err.Error())
		return
	}
	r.sess.PlayList(summariesToSessionTracks(p.Tracks), startIdx)
	if cur := r.sess.State().Current; cur != nil {
		r.loadAndBroadcast(cur.ID)
	}
}

// advance moves forward in the queue. If onDeadTrack, it counts as a skip.
func (r *CmdRouter) advance(onDeadTrack bool) {
	if onDeadTrack {
		_, ok, exhausted := r.sess.SkipDead()
		if exhausted {
			r.broadcastError(ErrKindQueueExhausted, "couldn't find a playable track in this queue")
			return
		}
		if !ok {
			return
		}
	} else {
		_, ok := r.sess.Next()
		if !ok {
			return
		}
	}
	cur := r.sess.State().Current
	if cur != nil {
		r.loadAndBroadcast(cur.ID)
	}
}

// loadAndBroadcast resolves a track, instructs the player to load it, and
// broadcasts the new state. On failure: skips and recurses with the cap.
func (r *CmdRouter) loadAndBroadcast(trackID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rt, err := r.res.Resolve(ctx, trackID)
	if err != nil {
		log.Printf("resolve %s: %v", trackID, err)
		r.broadcastError(ErrKindNotAvailable, "track unavailable: "+trackID)
		r.advance(true) // counts as skip
		return
	}
	streamURL := "/stream/" + trackID + "?t=" + r.token
	pb, _ := json.Marshal(LoadPayload{TrackID: trackID, StreamURL: streamURL})
	r.hub.SendToPlayer(Do{Type: "do", Kind: DoLoad, Payload: pb})
	r.sess.MarkLoaded()
	r.broadcastState()
	_ = rt // silence unused; presence verified by err==nil above
}

func (r *CmdRouter) broadcastState() {
	r.hub.Broadcast(StateUpdate{Type: "state", State: r.sess.State()})
}

func (r *CmdRouter) broadcastError(kind, msg string) {
	r.hub.Broadcast(ErrorMessage{Type: "error", Kind: kind, Message: msg})
}

func toSessionTrack(td *gateway.TrackData) session.Track {
	return session.Track{
		ID: td.SngID, Title: td.Title, Artist: td.Artist, Album: td.Album,
		DurationS: td.DurationS, CoverURL: CoverURL(td.CoverMD5),
	}
}

func summariesToSessionTracks(in []gateway.TrackSummary) []session.Track {
	out := make([]session.Track, 0, len(in))
	for _, t := range in {
		out = append(out, session.Track{
			ID: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album,
			DurationS: t.DurationS, CoverURL: CoverURL(t.CoverMD5),
		})
	}
	return out
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/transport/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/router.go internal/transport/router_test.go
git commit -m "feat(transport): command router

Translates controller cmds into session-state mutations + player do
instructions, drives auto-advance with 5-skip cap on ended playback,
and broadcasts state snapshots after every change."
```

---

### Task 19: HTTP server assembly + `web` embed

Mounts the API, /stream, /ws, and the embedded SPA. Returns an `*http.Server` so cmd/serve.go can `ListenAndServe`.

**Files:**
- Create: `internal/transport/server.go`
- Create: `internal/web/embed.go`

- [ ] **Step 1: Implement embedding stub**

The actual web bundle lands in Phase F; create a minimal stub so this task can build. Create `internal/web/embed.go`:

```go
// Package web exposes the embedded SPA bundle that lives under web/dist/.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded SPA filesystem rooted at web/dist/.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // compile-time invariant; only fires if //go:embed misses
	}
	return sub
}
```

Create a placeholder `web/dist/index.html` (Task 20 replaces it):

```bash
mkdir -p web/dist
printf '<!doctype html><meta charset="utf-8"><title>deezer-remote</title>\n' > web/dist/index.html
```

- [ ] **Step 2: Implement server assembly**

Create `internal/transport/server.go`:

```go
package transport

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/session"
	"github.com/niref/deezer-remote/internal/web"
)

// gwAdapter satisfies media.TrackFetcher over a gateway.Client. The shape
// mismatch is small enough to handle inline.
type gwAdapter struct {
	c *gateway.Client
	m *media.URLClient
}

// NewServer wires everything together and returns an *http.Server ready for
// ListenAndServe.
//
// The caller is responsible for: loading config, calling gw.GetUserData
// (to seed CSRF + license token), and shutting the server down.
type ServerInput struct {
	BindAddr     string
	BearerToken  string
	LicenseToken string
	Gateway      *gateway.Client
	URLClient    media.TrackURLAPI // see urlapi.go
	Cache        *media.Cache
	Sess         *session.Session
}

// NewServer builds the *http.Server. Returns the server and the hub so cmd
// code can introspect for logging.
func NewServer(in ServerInput) (*http.Server, *Hub) {
	resolver := media.NewResolver(
		&fetcherAdapter{c: in.Gateway, u: in.URLClient},
		in.LicenseToken,
		[]media.Format{media.FormatMP3_320, media.FormatMP3_128},
		in.Cache,
	)

	apiH := NewAPIHandlers(in.Gateway)
	streamH := NewStreamHandler(resolver, &http.Client{Timeout: 0})

	mux := http.NewServeMux()
	// Public, token-protected:
	apiMux := http.NewServeMux()
	apiH.Register(apiMux)
	streamH.Register(apiMux)
	mux.Handle("/api/", RequireToken(in.BearerToken)(apiMux))
	mux.Handle("/stream/", RequireToken(in.BearerToken)(apiMux))

	// WS endpoint:
	router := NewRouter(in.Sess, in.Gateway, resolver, nil, in.BearerToken) // hub injected next
	hub := NewHub(router)
	router.hub = hub // late-bind to break circular ctor
	mux.Handle("/ws", RequireToken(in.BearerToken)(hub))

	// Static SPA (no token gating; the page reads `?t=` from the URL on first
	// load and stores it in localStorage; subsequent /api / /ws calls carry it):
	mux.Handle("/", http.FileServerFS(web.FS()))

	return &http.Server{
		Addr:              in.BindAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}, hub
}

// fetcherAdapter satisfies media.TrackFetcher.
type fetcherAdapter struct {
	c *gateway.Client
	u media.TrackURLAPI
}

func (a *fetcherAdapter) SongGetData(ctx context.Context, trackID string) (media.TrackInfo, error) {
	td, err := a.c.SongGetData(ctx, trackID)
	if err != nil {
		return media.TrackInfo{}, err
	}
	return media.TrackInfo{
		SngID: td.SngID, TrackToken: td.TrackToken,
		FileSizeMP3_320: td.FileSizeMP3_320, FileSizeMP3_128: td.FileSizeMP3_128,
	}, nil
}

func (a *fetcherAdapter) GetURL(ctx context.Context, req media.URLRequest) (*media.URLResult, error) {
	return a.u.GetURL(ctx, req)
}

// Touchpoint for tests that need to inspect the embedded FS shape.
var _ fs.FS = web.FS()
```

`router.hub` is currently private. Make it settable: in `internal/transport/router.go`, change `hub HubSink` field from initialised in `NewRouter` to settable:

```go
// NewRouter wires a router. hub may be nil and set later via SetHub
// (cmd-layer wiring builds the hub after the router).
func NewRouter(sess *session.Session, gw GatewayAPI, res Resolver, hub HubSink, token string) *CmdRouter {
	return &CmdRouter{sess: sess, gw: gw, res: res, hub: hub, token: token}
}

// SetHub replaces the router's hub (used during server bootstrap).
func (r *CmdRouter) SetHub(h HubSink) { r.hub = h }
```

…and in `server.go` use `router.SetHub(hub)` instead of `router.hub = hub`.

Also rename `media.Client` to `media.URLClient` for clarity OR define a small alias in `media/url.go`:

```go
// In internal/media/url.go, after `type Client struct {...}`:

// URLClient is an alias for Client, kept so callers can be explicit about the
// role (Client is media-URL-specific despite the generic name).
type URLClient = Client

// TrackURLAPI is the subset of *Client the resolver needs. Letting callers
// pass an interface (instead of *Client) keeps tests cheap.
type TrackURLAPI interface {
	GetURL(ctx context.Context, req URLRequest) (*URLResult, error)
}
```

Add the import `"context"` if needed; it's already there.

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 4: Quick smoke test**

Run:
```bash
go test ./internal/transport/ -v
```
Expected: all tests still PASS (nothing new under test here).

- [ ] **Step 5: Commit**

```bash
git add internal/web/ web/dist/ internal/transport/server.go internal/transport/router.go internal/media/url.go
git commit -m "feat(transport): HTTP+WS server assembly with embedded SPA stub

NewServer composes API + /stream + /ws + static SPA over a single
mux. Bearer token middleware covers /api and /stream; /ws relies on
its own ?t= check via RequireToken. SPA bundle is embedded from
web/dist/ via internal/web."
```

---

## Phase F — Web UI (Alpine.js + plain JS)

The view layer is Alpine; state and side effects (WebSocket, fetch) live in plain JS modules so a future per-screen migration to Preact only touches templates.

### Task 20: Skeleton — HTML, CSS, vendored Alpine

**Files:**
- Modify: `web/dist/index.html`
- Create: `web/dist/styles.css`
- Create: `web/dist/vendor/alpine.min.js` (downloaded; not hand-written)

- [ ] **Step 1: Vendor Alpine 3**

Run:
```bash
mkdir -p web/dist/vendor
curl -sSL https://unpkg.com/alpinejs@3.13.10/dist/cdn.min.js -o web/dist/vendor/alpine.min.js
test -s web/dist/vendor/alpine.min.js  # confirm non-empty
```

- [ ] **Step 2: Replace `index.html` with the SPA shell**

Overwrite `web/dist/index.html` with:

```html
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <title>deezer-remote</title>
  <link rel="stylesheet" href="/styles.css">
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@tabler/icons-webfont@2.47.0/tabler-icons.min.css">
</head>
<body
  x-data="appRoot"
  x-init="$store.session.init(); window.__bootApp($store);"
  :class="role">

  <!-- PLAYER SHELL (laptop): just an <audio> tag + status -->
  <main class="player-shell" x-show="role === 'player'">
    <audio id="audio" preload="auto" x-ref="audio"></audio>
    <section class="player-card">
      <i class="ti ti-vinyl player-vinyl" aria-hidden="true"></i>
      <p class="player-title" x-text="$store.session.current?.title || 'Waiting for a track…'"></p>
      <p class="player-sub"   x-text="$store.session.current?.artist || ''"></p>
      <p class="player-hint">Use your phone to control playback.</p>
    </section>
  </main>

  <!-- CONTROLLER SHELL (phone): search → results → now-playing -->
  <main class="controller-shell" x-show="role === 'controller'">
    <header class="topbar">
      <span class="topbar-title">deezer-remote</span>
      <span class="topbar-status" x-text="$store.session.connected ? 'live' : 'reconnecting…'"></span>
    </header>

    <section class="search">
      <input
        type="search"
        class="search-input"
        placeholder="Search tracks, albums, playlists…"
        x-model.debounce.250ms="search.q"
        @input="search.run()"
        autocomplete="off"
        autofocus>
    </section>

    <section class="results" x-show="search.q">
      <template x-for="t in search.tracks" :key="t.id">
        <article class="row track" @click="playTrack(t)">
          <img class="row-cover" :src="t.cover_url" :alt="t.title" loading="lazy">
          <div class="row-text">
            <p class="row-title" x-text="t.title"></p>
            <p class="row-sub"   x-text="t.artist + ' · ' + t.album"></p>
          </div>
          <i class="ti ti-player-play-filled row-cta" aria-hidden="true"></i>
        </article>
      </template>
      <template x-for="a in search.albums" :key="'a' + a.id">
        <article class="row album" @click="playAlbum(a)">
          <img class="row-cover" :src="a.cover_url" :alt="a.title" loading="lazy">
          <div class="row-text">
            <p class="row-title" x-text="a.title"></p>
            <p class="row-sub"   x-text="'Album · ' + a.artist + ' · ' + a.track_count + ' tracks'"></p>
          </div>
          <i class="ti ti-player-play-filled row-cta" aria-hidden="true"></i>
        </article>
      </template>
      <template x-for="p in search.playlists" :key="'p' + p.id">
        <article class="row playlist" @click="playPlaylist(p)">
          <img class="row-cover" :src="p.cover_url" :alt="p.title" loading="lazy">
          <div class="row-text">
            <p class="row-title" x-text="p.title"></p>
            <p class="row-sub"   x-text="'Playlist · ' + p.owner + ' · ' + p.track_count + ' tracks'"></p>
          </div>
          <i class="ti ti-player-play-filled row-cta" aria-hidden="true"></i>
        </article>
      </template>
    </section>

    <section class="now-playing" x-show="$store.session.current && !search.q">
      <img class="np-cover" :src="$store.session.current?.cover_url" :alt="$store.session.current?.title">
      <p class="np-title" x-text="$store.session.current?.title"></p>
      <p class="np-sub"   x-text="$store.session.current?.artist + ' · ' + $store.session.current?.album"></p>

      <div class="seek">
        <div class="seek-bar" @click="seekAtClick($event)">
          <div class="seek-fill" :style="`width: ${seekPct()}%`"></div>
          <div class="seek-knob" :style="`left: ${seekPct()}%`"></div>
        </div>
        <div class="seek-labels">
          <span x-text="fmt($store.session.position_ms)"></span>
          <span x-text="fmt(($store.session.current?.duration_s || 0) * 1000)"></span>
        </div>
      </div>

      <div class="transport">
        <button class="t-skip" @click="cmd('prev')" aria-label="previous"><i class="ti ti-player-skip-back"></i></button>
        <button class="t-main" @click="togglePlay()" :aria-label="$store.session.paused ? 'play' : 'pause'">
          <i :class="$store.session.paused ? 'ti ti-player-play-filled' : 'ti ti-player-pause-filled'"></i>
        </button>
        <button class="t-skip" @click="cmd('next')" aria-label="next"><i class="ti ti-player-skip-forward"></i></button>
      </div>

      <div class="volume">
        <i class="ti ti-volume-3" aria-hidden="true"></i>
        <input type="range" min="0" max="100" :value="Math.round($store.session.volume * 100)"
               @input="setVolume($event.target.value / 100)">
        <i class="ti ti-volume" aria-hidden="true"></i>
      </div>
    </section>

    <section class="errors" x-show="$store.ui.errors.length">
      <template x-for="e in $store.ui.errors" :key="e.id">
        <div class="toast" :class="'kind-' + e.kind" x-text="e.message"></div>
      </template>
    </section>
  </main>

  <script src="/vendor/alpine.min.js" defer></script>
  <script type="module" src="/js/app.js"></script>
</body>
</html>
```

- [ ] **Step 3: Implement styles**

Create `web/dist/styles.css`:

```css
:root {
  --bg: #0F0F10;
  --bg2: #1A1A1B;
  --fg: #F2F2F2;
  --fg-muted: #A0A0A0;
  --fg-dim: #777;
  --accent: #EF5466;
  --accent-soft: rgba(239,84,102,0.12);
  --line: #2A2A2A;
  --radius: 16px;
  --radius-sm: 10px;
}

* { box-sizing: border-box; }
html, body { margin: 0; padding: 0; background: var(--bg); color: var(--fg);
  font-family: system-ui, -apple-system, Segoe UI, Roboto, Inter, sans-serif;
  font-size: 16px; }
body { min-height: 100dvh; }

/* --- Player shell (laptop) --- */
.player-shell { display: flex; align-items: center; justify-content: center;
  min-height: 100dvh; padding: 2rem; }
.player-card { background: var(--bg2); border-radius: var(--radius);
  padding: 2.5rem 2rem; text-align: center; max-width: 380px; width: 100%;
  border: 1px solid var(--line); }
.player-vinyl { font-size: 96px; color: var(--accent); display: block; margin: 0 auto 1rem; }
.player-title { margin: 0; font-size: 18px; font-weight: 500; }
.player-sub { margin: 4px 0 1.5rem; color: var(--fg-muted); font-size: 14px; }
.player-hint { margin: 0; color: var(--fg-dim); font-size: 13px; }

/* --- Controller shell (phone) --- */
.controller-shell { padding: env(safe-area-inset-top, 12px) 16px 32px; max-width: 480px; margin: 0 auto; }
.topbar { display: flex; justify-content: space-between; align-items: baseline; margin: 8px 4px 16px; }
.topbar-title { font-weight: 600; font-size: 14px; letter-spacing: 0.5px; text-transform: uppercase; color: var(--fg-muted); }
.topbar-status { font-size: 11px; color: var(--fg-dim); }

.search { margin-bottom: 16px; }
.search-input { width: 100%; padding: 12px 14px; border-radius: var(--radius-sm);
  background: var(--bg2); border: 1px solid var(--line); color: var(--fg);
  font-size: 16px; outline: none; }
.search-input:focus { border-color: var(--accent); }

.results { display: flex; flex-direction: column; gap: 8px; }
.row { display: flex; align-items: center; gap: 12px; padding: 8px;
  border-radius: var(--radius-sm); border: 1px solid var(--line); background: var(--bg2);
  cursor: pointer; transition: background 0.15s; }
.row:active { background: var(--accent-soft); }
.row-cover { width: 48px; height: 48px; border-radius: 6px; background: var(--bg);
  object-fit: cover; flex-shrink: 0; }
.row-text { flex: 1; min-width: 0; }
.row-title { margin: 0; font-size: 14px; font-weight: 500; overflow: hidden;
  text-overflow: ellipsis; white-space: nowrap; }
.row-sub { margin: 2px 0 0; font-size: 12px; color: var(--fg-muted);
  overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.row-cta { font-size: 22px; color: var(--accent); }

.now-playing { margin-top: 24px; text-align: center; }
.np-cover { width: 100%; max-width: 280px; aspect-ratio: 1; border-radius: var(--radius);
  margin: 0 auto 16px; object-fit: cover; background: var(--bg2); }
.np-title { margin: 0; font-size: 18px; font-weight: 500; }
.np-sub { margin: 4px 0 20px; font-size: 13px; color: var(--fg-muted); }

.seek { margin: 0 0 16px; }
.seek-bar { position: relative; height: 4px; background: var(--line); border-radius: 2px; cursor: pointer; }
.seek-fill { position: absolute; left: 0; top: 0; height: 100%; background: var(--accent); border-radius: 2px; }
.seek-knob { position: absolute; top: 50%; transform: translate(-50%, -50%); width: 12px;
  height: 12px; background: var(--accent); border-radius: 50%; }
.seek-labels { display: flex; justify-content: space-between; margin-top: 8px;
  font-size: 12px; color: var(--fg-dim); font-variant-numeric: tabular-nums; }

.transport { display: flex; align-items: center; justify-content: center; gap: 32px; margin: 8px 0 20px; }
.transport button { background: none; border: 0; color: var(--fg); cursor: pointer; padding: 8px; }
.t-skip i { font-size: 32px; }
.t-main { background: var(--accent); border-radius: 50%; width: 64px; height: 64px;
  display: flex; align-items: center; justify-content: center; color: white; }
.t-main i { font-size: 28px; }

.volume { display: flex; align-items: center; gap: 12px; }
.volume i { font-size: 16px; color: var(--fg-dim); }
.volume input[type=range] { flex: 1; accent-color: var(--fg); }

.errors { position: fixed; bottom: 16px; left: 16px; right: 16px; display: flex;
  flex-direction: column; gap: 8px; pointer-events: none; }
.toast { background: var(--bg2); border: 1px solid var(--line); padding: 10px 14px;
  border-radius: var(--radius-sm); font-size: 13px; }
.toast.kind-auth_expired,
.toast.kind-queue_exhausted { border-color: var(--accent); }

/* Desktop-only adjustments for the controller shell (rare but harmless). */
@media (min-width: 700px) {
  .controller-shell { padding-top: 24px; }
}
```

- [ ] **Step 4: Build (verifies //go:embed picks up the new files)**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add web/dist/index.html web/dist/styles.css web/dist/vendor/alpine.min.js
git commit -m "feat(web): SPA shell, mockup-fidelity styles, vendored Alpine

Single index.html that switches between player and controller layouts
based on viewport / role. Tabler icons via CDN (no Node build).
Alpine.js 3 vendored under web/dist/vendor/ — embedded via //go:embed
all:dist so the binary is fully self-contained."
```

---

### Task 21: WebSocket client + Alpine stores + API helpers

**Files:**
- Create: `web/dist/js/api.js`
- Create: `web/dist/js/ws.js`
- Create: `web/dist/js/stores.js`

- [ ] **Step 1: Implement `api.js`**

Create `web/dist/js/api.js`:

```js
// Thin /api/* fetch helpers. The bearer token is read from localStorage
// (stored by app.js on first load).
const TOKEN_KEY = "deezer-remote/token";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || "";
}

export function setToken(t) {
  localStorage.setItem(TOKEN_KEY, t);
}

async function get(path) {
  const r = await fetch(path, {
    headers: { Authorization: `Bearer ${getToken()}` },
  });
  if (!r.ok) throw new Error(`${path}: HTTP ${r.status}`);
  return r.json();
}

export const api = {
  search: (q) => get(`/api/search?q=${encodeURIComponent(q)}`),
  track:  (id) => get(`/api/track/${encodeURIComponent(id)}`),
  album:  (id) => get(`/api/album/${encodeURIComponent(id)}`),
  playlist: (id) => get(`/api/playlist/${encodeURIComponent(id)}`),
};
```

- [ ] **Step 2: Implement `ws.js`**

Create `web/dist/js/ws.js`:

```js
// Reconnecting WebSocket client. Exposes events via a tiny pub/sub.
import { getToken } from "./api.js";

const RECONNECT_MS = [500, 1000, 2000, 5000, 10000];

export class WSClient {
  constructor() {
    this.ws = null;
    this.attempt = 0;
    this.handlers = new Map(); // type -> Set<fn>
    this.role = null;
    this.connected = false;
  }

  connect(role) {
    this.role = role;
    this._open();
  }

  _open() {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const url = `${proto}://${location.host}/ws?t=${encodeURIComponent(getToken())}`;
    const ws = new WebSocket(url);
    this.ws = ws;

    ws.addEventListener("open", () => {
      this.attempt = 0;
      this.connected = true;
      this._emit("__open", null);
      this.send({ type: "hello", role: this.role });
    });

    ws.addEventListener("message", (ev) => {
      let msg; try { msg = JSON.parse(ev.data); } catch { return; }
      this._emit(msg.type, msg);
    });

    ws.addEventListener("close", () => {
      this.connected = false;
      this._emit("__close", null);
      const delay = RECONNECT_MS[Math.min(this.attempt, RECONNECT_MS.length - 1)];
      this.attempt++;
      setTimeout(() => this._open(), delay);
    });

    ws.addEventListener("error", () => { /* close fires after */ });
  }

  send(obj) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(obj));
    }
  }

  on(type, fn) {
    let set = this.handlers.get(type);
    if (!set) { set = new Set(); this.handlers.set(type, set); }
    set.add(fn);
    return () => set.delete(fn);
  }

  _emit(type, msg) {
    const set = this.handlers.get(type);
    if (set) for (const fn of set) fn(msg);
  }
}

export const ws = new WSClient();
```

- [ ] **Step 3: Implement `stores.js`**

Create `web/dist/js/stores.js`:

```js
// Alpine stores: session and ui. State mutations live here so a future
// migration off Alpine (per-screen Preact, say) only requires re-binding
// templates — these modules don't change.
import { ws } from "./ws.js";

export function registerStores(Alpine) {
  Alpine.store("session", {
    connected: false,
    current: null,
    queue: [],
    queue_pos: 0,
    position_ms: 0,
    paused: true,
    volume: 1.0,

    init() {
      ws.on("__open",  () => { this.connected = true; });
      ws.on("__close", () => { this.connected = false; });
      ws.on("state", (m) => {
        const s = m.state || {};
        this.current     = s.current || null;
        this.queue       = s.queue || [];
        this.queue_pos   = s.queue_pos | 0;
        this.position_ms = s.position_ms | 0;
        this.paused      = !!s.paused;
        this.volume      = typeof s.volume === "number" ? s.volume : 1.0;
      });
      ws.on("error", (m) => {
        Alpine.store("ui").pushError(m);
      });
    },
  });

  Alpine.store("ui", {
    errors: [],
    _nextId: 1,
    pushError(m) {
      const e = { id: this._nextId++, kind: m.kind, message: m.message };
      this.errors.push(e);
      setTimeout(() => {
        const i = this.errors.findIndex(x => x.id === e.id);
        if (i >= 0) this.errors.splice(i, 1);
      }, 5000);
    },
  });
}
```

- [ ] **Step 4: Build (the new files are picked up by //go:embed)**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add web/dist/js/api.js web/dist/js/ws.js web/dist/js/stores.js
git commit -m "feat(web): api helpers, reconnecting WS client, Alpine stores"
```

---

### Task 22: Player module — `<audio>` control + playback push

**Files:**
- Create: `web/dist/js/player.js`

- [ ] **Step 1: Implement**

Create `web/dist/js/player.js`:

```js
// Player tab: wraps the <audio> element, executes do-instructions from the
// service, and pushes playback updates every ~250 ms.
import { ws } from "./ws.js";

const PUSH_INTERVAL_MS = 250;

export function attachPlayer(audio) {
  let lastPushed = 0;

  function pushNow() {
    ws.send({
      type: "playback",
      position_ms: Math.round((audio.currentTime || 0) * 1000),
      paused: audio.paused,
      ended: false,
    });
    lastPushed = performance.now();
  }

  setInterval(() => {
    if (performance.now() - lastPushed >= PUSH_INTERVAL_MS) pushNow();
  }, PUSH_INTERVAL_MS);

  audio.addEventListener("play",  pushNow);
  audio.addEventListener("pause", pushNow);
  audio.addEventListener("seeked", pushNow);
  audio.addEventListener("ended", () => {
    ws.send({
      type: "playback",
      position_ms: Math.round((audio.duration || 0) * 1000),
      paused: true,
      ended: true,
    });
  });

  ws.on("do", (m) => {
    switch (m.kind) {
      case "load":
        audio.src = m.payload.stream_url;
        audio.play().catch(err => console.warn("audio.play:", err));
        break;
      case "play":
        audio.play().catch(err => console.warn("audio.play:", err));
        break;
      case "pause":
        audio.pause();
        break;
      case "seek":
        audio.currentTime = (m.payload.position_ms || 0) / 1000;
        break;
      case "set_volume":
        audio.volume = Math.max(0, Math.min(1, m.payload.volume || 0));
        break;
    }
  });
}
```

- [ ] **Step 2: Commit**

```bash
git add web/dist/js/player.js
git commit -m "feat(web): player module — audio control + 250ms playback push"
```

---

### Task 23: Controller module + app bootstrap

`controller.js` exposes the Alpine component (`appRoot`) that the controller HTML binds to. `app.js` is the entry point that imports everything, registers Alpine plugins, and selects the role.

**Files:**
- Create: `web/dist/js/controller.js`
- Create: `web/dist/js/app.js`

- [ ] **Step 1: Implement `controller.js`**

Create `web/dist/js/controller.js`:

```js
// Controller-tab Alpine component. Holds search state and per-tab UI helpers.
// Mutations to shared session state flow through cmds → service → state
// broadcast; this file never mutates Alpine.store('session') directly.
import { api } from "./api.js";
import { ws } from "./ws.js";

export function appRoot() {
  return {
    role: detectRole(),
    search: {
      q: "",
      tracks: [],
      albums: [],
      playlists: [],
      _seq: 0,
      async run() {
        const q = this.q.trim();
        if (!q) {
          this.tracks = []; this.albums = []; this.playlists = [];
          return;
        }
        const my = ++this._seq;
        try {
          const r = await api.search(q);
          if (my !== this._seq) return; // a newer query landed
          this.tracks = r.tracks || [];
          this.albums = r.albums || [];
          this.playlists = r.playlists || [];
        } catch (err) {
          console.warn("search:", err);
        }
      },
    },

    playTrack(t) {
      ws.send({ type: "cmd", kind: "play_track", payload: { track_id: t.id } });
      this.search.q = "";
    },
    playAlbum(a) {
      ws.send({ type: "cmd", kind: "play_album", payload: { album_id: a.id, start_idx: 0 } });
      this.search.q = "";
    },
    playPlaylist(p) {
      ws.send({ type: "cmd", kind: "play_playlist", payload: { playlist_id: p.id, start_idx: 0 } });
      this.search.q = "";
    },
    cmd(kind) { ws.send({ type: "cmd", kind }); },
    togglePlay() {
      ws.send({ type: "cmd", kind: this.$store.session.paused ? "play" : "pause" });
    },
    setVolume(v) {
      ws.send({ type: "cmd", kind: "set_volume", payload: { volume: v } });
    },
    seekAtClick(ev) {
      const bar = ev.currentTarget;
      const rect = bar.getBoundingClientRect();
      const pct = Math.max(0, Math.min(1, (ev.clientX - rect.left) / rect.width));
      const dur = (this.$store.session.current?.duration_s || 0) * 1000;
      const pos = Math.round(pct * dur);
      ws.send({ type: "cmd", kind: "seek", payload: { position_ms: pos } });
    },
    seekPct() {
      const dur = (this.$store.session.current?.duration_s || 0) * 1000;
      if (!dur) return 0;
      return Math.min(100, (this.$store.session.position_ms / dur) * 100);
    },
    fmt(ms) {
      ms = Math.max(0, ms | 0);
      const s = Math.floor(ms / 1000);
      const m = Math.floor(s / 60);
      const ss = (s % 60).toString().padStart(2, "0");
      return `${m}:${ss}`;
    },
  };
}

// detectRole: query string ?role=player|controller pins it; otherwise use
// viewport width as a heuristic (wide = laptop player, narrow = phone
// controller). The user can also bookmark with ?role= to lock it.
function detectRole() {
  const q = new URLSearchParams(location.search).get("role");
  if (q === "player" || q === "controller") return q;
  return window.matchMedia("(min-width: 700px) and (pointer: fine)").matches
    ? "player" : "controller";
}
```

- [ ] **Step 2: Implement `app.js`**

Create `web/dist/js/app.js`:

```js
// Bootstrap: strip token from URL on first load, register Alpine components
// and stores, connect the WS, and wire the audio element if we're in player
// role.
import { setToken, getToken } from "./api.js";
import { ws } from "./ws.js";
import { registerStores } from "./stores.js";
import { appRoot } from "./controller.js";
import { attachPlayer } from "./player.js";

(function bootToken() {
  const url = new URL(location.href);
  const t = url.searchParams.get("t");
  if (t) {
    setToken(t);
    url.searchParams.delete("t");
    history.replaceState({}, "", url.toString());
  }
})();

document.addEventListener("alpine:init", () => {
  window.Alpine.data("appRoot", appRoot);
  registerStores(window.Alpine);
});

window.__bootApp = function (store) {
  if (!getToken()) {
    console.error("no bearer token; re-open the URL with ?t= or run `deezer-remote pair`");
    return;
  }
  const role = document.body.classList.contains("player") ? "player" : "controller";
  // Note: body.classList isn't set yet at this point — use the heuristic.
  const detected = window.matchMedia("(min-width: 700px) and (pointer: fine)").matches
    ? "player" : "controller";
  const finalRole = new URLSearchParams(location.search).get("role") || detected;
  ws.connect(finalRole);
  if (finalRole === "player") {
    const audio = document.getElementById("audio");
    attachPlayer(audio);
  }
};
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add web/dist/js/controller.js web/dist/js/app.js
git commit -m "feat(web): controller component + app bootstrap

Role auto-detection by viewport (override with ?role=player|controller).
Token is read from ?t= on first load, stored in localStorage, then
scrubbed from the address bar so bookmarking is clean."
```

---

## Phase G — CLI

### Task 24: Cobra root command

**Files:**
- Create: `cmd/deezer-remote/main.go`

- [ ] **Step 1: Add cobra**

Run: `go get github.com/spf13/cobra@latest`

- [ ] **Step 2: Implement**

Create `cmd/deezer-remote/main.go`:

```go
// Command deezer-remote is the on-LAN remote-control music app:
//
//	deezer-remote serve     # start the HTTP + WS service
//	deezer-remote pair      # print pairing URLs + QR (no rotation)
//	deezer-remote pair --reset    # rotate token then print
//	deezer-remote doctor    # validate setup
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is overridable at link time: -ldflags "-X main.Version=...".
var Version = "0.1.0-dev"

func main() {
	root := &cobra.Command{
		Use:           "deezer-remote",
		Short:         "On-LAN remote-control music app for Deezer.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(serveCmd())
	root.AddCommand(pairCmd())
	root.AddCommand(doctorCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 3: Tidy + build**

The subcommand files are next; create stub functions so this compiles. In a new file `cmd/deezer-remote/stubs.go`:

```go
package main

import "github.com/spf13/cobra"

func serveCmd() *cobra.Command  { return &cobra.Command{Use: "serve",  RunE: notImplemented} }
func pairCmd() *cobra.Command   { return &cobra.Command{Use: "pair",   RunE: notImplemented} }
func doctorCmd() *cobra.Command { return &cobra.Command{Use: "doctor", RunE: notImplemented} }

func notImplemented(*cobra.Command, []string) error { return nil }
```

Run:
```bash
go mod tidy
go build ./...
```
Expected: clean. `deezer-remote --help` should list the subcommands.

- [ ] **Step 4: Commit**

```bash
git add cmd/deezer-remote/ go.mod go.sum
git commit -m "feat(cmd): cobra root with serve / pair / doctor stubs"
```

---

### Task 25: `pair` subcommand

Prints player URL (`http://localhost:8080/?t=<token>`) and phone URL with a QR code. `--reset` rotates the token before printing.

**Files:**
- Modify: `cmd/deezer-remote/stubs.go` (remove `pairCmd` stub)
- Create: `cmd/deezer-remote/pair.go`

- [ ] **Step 1: Implement**

Delete the `pairCmd` line from `cmd/deezer-remote/stubs.go` so it lives only in `pair.go`.

Create `cmd/deezer-remote/pair.go`:

```go
package main

import (
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/network"
	"github.com/niref/deezer-remote/internal/qrterm"
)

func pairCmd() *cobra.Command {
	var reset bool
	var port int
	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Print pairing URLs (player + phone QR). Use --reset to rotate the token.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPair(reset, port)
		},
	}
	cmd.Flags().BoolVar(&reset, "reset", false, "rotate the bearer token (invalidates paired devices)")
	cmd.Flags().IntVar(&port, "port", 8080, "service port (matches `serve --port`)")
	return cmd
}

func runPair(reset bool, port int) error {
	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.LoadFromPathExported(path)
	if err != nil {
		return err
	}
	var tok string
	if reset {
		tok, err = config.RotateToken(path, cfg)
	} else {
		tok, err = config.EnsureToken(path, cfg)
	}
	if err != nil {
		return err
	}

	addrs, err := network.LANAddrs()
	if err != nil {
		return err
	}

	playerURL := "http://localhost:" + strconv.Itoa(port) + "/?t=" + tok
	fmt.Println("Player URL (open on the laptop):")
	fmt.Println("  " + playerURL)
	fmt.Println()

	if len(addrs) == 0 {
		fmt.Println("⚠ No private (RFC1918) LAN address found. Phone pairing won't work.")
		return nil
	}
	fmt.Println("Phone URL (scan QR with phone camera):")
	for _, a := range addrs {
		phoneURL := buildURL(a, port, tok)
		fmt.Println("  " + phoneURL)
		qrterm.Render(os.Stdout, phoneURL)
		fmt.Println()
	}
	return nil
}

func buildURL(ip net.IP, port int, tok string) string {
	return "http://" + ip.String() + ":" + strconv.Itoa(port) + "/?t=" + tok
}
```

`config.LoadFromPathExported` doesn't exist yet — expose `loadFromPath` via a small thin wrapper. In `internal/config/config.go`, add:

```go
// LoadFromPath reads a config file at an explicit path. Used by the pair
// and doctor subcommands so they can operate on the same file Load uses.
func LoadFromPath(path string) (*Config, error) { return loadFromPath(path) }
```

…and update `pair.go` to call `config.LoadFromPath(path)` (rename in the file above).

- [ ] **Step 2: Build + smoke test**

Run:
```bash
go build ./...
./deezer-remote pair --help
```
Expected: help renders cleanly.

(A full `pair` run requires an existing arl in `~/.config/deezer-remote/config.toml` and will print a QR. Don't include the QR output in the commit body.)

- [ ] **Step 3: Commit**

```bash
git add cmd/deezer-remote/pair.go cmd/deezer-remote/stubs.go internal/config/config.go
git commit -m "feat(cmd): pair subcommand prints player URL + phone QR

--reset rotates the bearer token (invalidates paired devices).
LANAddrs filters to RFC1918 only; we print all candidates so the user
can pick if their laptop has multiple interfaces."
```

---

### Task 26: `serve` subcommand

Wires config → gateway → media → session → transport.Server → http.Server.

**Files:**
- Modify: `cmd/deezer-remote/stubs.go` (remove `serveCmd` stub)
- Create: `cmd/deezer-remote/serve.go`

- [ ] **Step 1: Implement**

Delete the `serveCmd` line from `stubs.go`.

Create `cmd/deezer-remote/serve.go`:

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/network"
	"github.com/niref/deezer-remote/internal/qrterm"
	"github.com/niref/deezer-remote/internal/session"
	"github.com/niref/deezer-remote/internal/transport"
)

func serveCmd() *cobra.Command {
	var (
		bindHost string
		port     int
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP + WebSocket service.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(bindHost, port)
		},
	}
	cmd.Flags().StringVar(&bindHost, "bind", "0.0.0.0", "host to bind (0.0.0.0 = all interfaces)")
	cmd.Flags().IntVar(&port, "port", 8080, "port to listen on")
	return cmd
}

func runServe(bindHost string, port int) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	path, err := config.DefaultPath()
	if err != nil {
		return err
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	tok, err := config.EnsureToken(path, cfg)
	if err != nil {
		return err
	}

	gw, err := gateway.NewClient(cfg.ARL)
	if err != nil {
		return err
	}
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		return fmt.Errorf("authenticate with Deezer (arl): %w", err)
	}
	fmt.Fprintf(os.Stderr, "Authenticated as user %d\n", ud.UserID)

	mc := media.NewClient(http.DefaultTransport)
	cache := media.NewCache(30 * time.Minute)
	sess := session.New()

	srv, _ := transport.NewServer(transport.ServerInput{
		BindAddr:     bindHost + ":" + strconv.Itoa(port),
		BearerToken:  tok,
		LicenseToken: ud.LicenseToken,
		Gateway:      gw,
		URLClient:    mc,
		Cache:        cache,
		Sess:         sess,
	})

	printURLs(port, tok)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(os.Stderr, "Listening on %s\n", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func printURLs(port int, tok string) {
	fmt.Println("Player URL (open on the laptop):")
	fmt.Println("  http://localhost:" + strconv.Itoa(port) + "/?t=" + tok)
	fmt.Println()
	addrs, _ := network.LANAddrs()
	if len(addrs) == 0 {
		fmt.Println("⚠ No private LAN address detected. Phone pairing won't work.")
		return
	}
	fmt.Println("Phone URL (scan QR with phone camera):")
	for _, a := range addrs {
		url := "http://" + a.String() + ":" + strconv.Itoa(port) + "/?t=" + tok
		fmt.Println("  " + url)
		qrterm.Render(os.Stdout, url)
		fmt.Println()
	}
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 3: Smoke run (manual)**

Run: `./deezer-remote serve --port 18080 &`. Hit `http://localhost:18080/` in a browser — should serve `index.html` (token-less request will load assets but WS will reject; that's fine).
Stop: `kill %1`.

- [ ] **Step 4: Commit**

```bash
git add cmd/deezer-remote/serve.go cmd/deezer-remote/stubs.go
git commit -m "feat(cmd): serve subcommand

Wires config + gateway + media + session + transport into an
http.Server. Authenticates the arl at startup (one getUserData call)
so an expired token fails fast rather than at first /api request."
```

---

### Task 27: `doctor` subcommand

Per spec: arl present, bearer exists, can bind port, every LAN IP accepts loopback connection, a configurable test track is fetchable end-to-end through `/stream/<id>` (range-bounded to first 64 KB).

**Files:**
- Modify: `cmd/deezer-remote/stubs.go` (remove `doctorCmd` stub)
- Create: `cmd/deezer-remote/doctor.go`

- [ ] **Step 1: Implement**

Delete the `doctorCmd` line from `stubs.go`.

Create `cmd/deezer-remote/doctor.go`:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/network"
	"github.com/niref/deezer-remote/internal/session"
	"github.com/niref/deezer-remote/internal/transport"
)

func doctorCmd() *cobra.Command {
	var (
		port    int
		trackID string
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate the setup (arl, token, port, LAN IPs, end-to-end stream).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(port, trackID)
		},
	}
	cmd.Flags().IntVar(&port, "port", 8080, "port to probe (matches `serve --port`)")
	cmd.Flags().StringVar(&trackID, "track", "3135556", "stable public track ID for the e2e probe")
	return cmd
}

type check struct {
	name string
	run  func(ctx context.Context) error
}

func runDoctor(port int, trackID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var path string
	var cfg *config.Config
	var gw *gateway.Client

	checks := []check{
		{"config file path", func(ctx context.Context) error {
			p, err := config.DefaultPath()
			path = p
			return err
		}},
		{"arl present (config readable, arl non-empty)", func(ctx context.Context) error {
			c, err := config.LoadFromPath(path)
			cfg = c
			return err
		}},
		{"bearer token present", func(ctx context.Context) error {
			if cfg.BearerToken == "" {
				return fmt.Errorf("no bearer token; run `deezer-remote pair`")
			}
			return nil
		}},
		{"can bind port " + strconv.Itoa(port), func(ctx context.Context) error {
			ln, err := net.Listen("tcp", "0.0.0.0:"+strconv.Itoa(port))
			if err != nil {
				return err
			}
			return ln.Close()
		}},
		{"arl authenticates against Deezer", func(ctx context.Context) error {
			c, err := gateway.NewClient(cfg.ARL)
			if err != nil {
				return err
			}
			gw = c
			ud, err := gw.GetUserData(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "  → user_id=%d\n", ud.UserID)
			return nil
		}},
		{"LAN addresses reachable from self", func(ctx context.Context) error {
			addrs, err := network.LANAddrs()
			if err != nil {
				return err
			}
			if len(addrs) == 0 {
				return fmt.Errorf("no private (RFC1918) interface")
			}
			for _, a := range addrs {
				if err := probeTCP(ctx, a.String()+":"+strconv.Itoa(port)); err != nil {
					fmt.Fprintf(os.Stderr, "  ⚠ %s: %v (firewall? wrong network profile?)\n", a, err)
				} else {
					fmt.Fprintf(os.Stderr, "  → %s reachable\n", a)
				}
			}
			return nil
		}},
		{"/stream e2e (track " + trackID + ", first 64 KB)", func(ctx context.Context) error {
			return probeStream(ctx, gw, cfg.BearerToken, trackID)
		}},
	}

	failed := 0
	for _, c := range checks {
		fmt.Fprintf(os.Stderr, "▶ %s\n", c.name)
		if err := c.run(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s\n", err)
			failed++
		} else {
			fmt.Fprintln(os.Stderr, "  ✓")
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d checks failed", failed)
	}
	fmt.Fprintln(os.Stderr, "All checks passed.")
	return nil
}

func probeTCP(ctx context.Context, hostport string) error {
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", hostport)
	if err != nil {
		return err
	}
	return conn.Close()
}

// probeStream stands up the transport pipeline against an in-process
// httptest-like config, then issues a Range:0-65535 request against
// /stream/<id>?t=<token> and verifies a 206 response with the right
// Content-Range.
func probeStream(ctx context.Context, gw *gateway.Client, tok, trackID string) error {
	mc := media.NewClient(http.DefaultTransport)
	cache := media.NewCache(30 * time.Minute)
	sess := session.New()
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		return err
	}
	srv, _ := transport.NewServer(transport.ServerInput{
		BindAddr:     "127.0.0.1:0",
		BearerToken:  tok,
		LicenseToken: ud.LicenseToken,
		Gateway:      gw,
		URLClient:    mc,
		Cache:        cache,
		Sess:         sess,
	})
	// Listen on an ephemeral port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() { _ = srv.Serve(ln) }()
	defer srv.Shutdown(context.Background())

	url := "http://" + ln.Addr().String() + "/stream/" + trackID + "?t=" + tok
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Range", "bytes=0-65535")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("expected 206, got %d", resp.StatusCode)
	}
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}
	if n < 4 {
		return fmt.Errorf("stream returned only %d bytes", n)
	}
	return nil
}
```

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: clean.

- [ ] **Step 3: Optional smoke run**

Run: `./deezer-remote doctor --track 3135556` (requires a working arl). All checks should pass — if not, the failure output lists which step failed and why.

- [ ] **Step 4: Commit**

```bash
git add cmd/deezer-remote/doctor.go cmd/deezer-remote/stubs.go
git commit -m "feat(cmd): doctor subcommand

Runs each spec'd check in order, prints a one-line hint per failure,
and exits non-zero on any failure. The e2e /stream probe stands up the
transport pipeline against an ephemeral 127.0.0.1 listener and issues
a Range request for the first 64 KB."
```

After this commit `stubs.go` should be empty or only contain unused fallbacks. Delete it:

```bash
git rm cmd/deezer-remote/stubs.go
git commit -m "chore(cmd): drop stub file now that all subcommands are implemented"
```

---

## Phase H — Validation & cleanup

### Task 28: Live integration test

Linux-only, gated. Reads the real `arl` and hits Deezer end-to-end (HEAD-only — does not download bytes from the CDN, to be polite).

**Files:**
- Create: `internal/media/integration_test.go`

- [ ] **Step 1: Implement**

Create `internal/media/integration_test.go`:

```go
//go:build linux

package media

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/niref/deezer-remote/internal/config"
	"github.com/niref/deezer-remote/internal/gateway"
)

// TestIntegration_GetMediaURL runs only when DEEZER_INTEGRATION=1.
// Verifies the live pipeline (arl auth → song.getData → media.getUrl)
// resolves a stable public track and that the returned URL is reachable
// (HEAD only — does not download audio bytes from the CDN).
func TestIntegration_GetMediaURL(t *testing.T) {
	if os.Getenv("DEEZER_INTEGRATION") != "1" {
		t.Skip("set DEEZER_INTEGRATION=1 to run")
	}
	const trackID = "3135556" // Daft Punk – Harder, Better, Faster, Stronger

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	path, err := config.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	gw, err := gateway.NewClient(cfg.ARL)
	if err != nil {
		t.Fatal(err)
	}
	ud, err := gw.GetUserData(ctx)
	if err != nil {
		t.Fatalf("getUserData: %v", err)
	}
	td, err := gw.SongGetData(ctx, trackID)
	if err != nil {
		t.Fatalf("song.getData: %v", err)
	}

	mc := NewClient(http.DefaultTransport)
	res, err := mc.GetURL(ctx, URLRequest{
		LicenseToken: ud.LicenseToken,
		TrackToken:   td.TrackToken,
		Formats:      []Format{FormatMP3_320, FormatMP3_128},
	})
	if err != nil {
		t.Fatalf("media.getUrl: %v", err)
	}
	if res.URL == "" {
		t.Fatal("empty media URL")
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, res.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HEAD CDN URL: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("CDN HEAD status = %d (URL may have changed semantics)", resp.StatusCode)
	}
	t.Logf("format=%s url=%.60s… exp=%d", res.Format, res.URL, res.Expiry)
}
```

- [ ] **Step 2: Verify gating**

Run: `go test ./internal/media/ -v`
Expected: PASS — the integration test SKIPs without the env var.

Optional (requires arl): `DEEZER_INTEGRATION=1 go test ./internal/media/ -run TestIntegration -v`.

- [ ] **Step 3: Commit**

```bash
git add internal/media/integration_test.go
git commit -m "test(media): live integration test gated by DEEZER_INTEGRATION=1

Validates arl auth + media.getUrl + CDN reachability against a stable
public track. HEAD-only on the CDN — does not download audio bytes."
```

---

### Task 29: Smoke checklist, delete spike, final build

**Files:**
- Delete: `cmd/spike/`
- Update: `docs/superpowers/specs/2026-05-13-deezer-remote-design.md` (mark Phase 1 done in TODO)
- Optional: `README.md`

- [ ] **Step 1: Delete the spike**

```bash
git rm -r cmd/spike
```

- [ ] **Step 2: Final full verification**

Run:
```bash
go mod tidy
go vet ./...
go build ./...
go test -race ./...
```
Expected: clean across the board.

- [ ] **Step 3: Mark the plan as executed in the spec**

Edit `docs/superpowers/specs/2026-05-13-deezer-remote-design.md`, replace the Phase 1 line in the TODO list:

```
- [x] Write the Phase 1 implementation plan (writing-plans skill).
- [x] Implement Phase 1 (this plan).
```

- [ ] **Step 4: Manual smoke run on the actual laptop**

Per the spec's manual smoke test plan:
1. `./deezer-remote pair` — QR + URL print.
2. `./deezer-remote serve` — server starts, prints URLs.
3. Open the player URL in the laptop browser; tab opens, status reads "Waiting for a track…".
4. Open the phone URL on the phone; controller UI appears.
5. Search "Daft Punk"; tap a track — playback starts on the laptop within 2 s.
6. Pause / resume / seek to 50% / skip / change volume from the phone; the laptop reflects each within 250 ms.
7. Close the player tab; phone toast appears within ~30 s ("desktop disconnected" via heartbeat). Reopen player; playback resumes from the saved position within 5 s.
8. `./deezer-remote doctor` — all green.

Document anything that doesn't behave as expected in a follow-up issue rather than patching in this PR.

- [ ] **Step 5: Commit + final note**

```bash
git add docs/superpowers/specs/2026-05-13-deezer-remote-design.md cmd/spike
git commit -m "chore: complete Phase 1; remove cmd/spike

The spike served its purpose (proving the streaming pipeline). All
spike behavior is now reachable via deezer-remote doctor --track <id>
and the integration test, so the throwaway is no longer needed."
```

---

## Self-Review Summary

**Spec coverage check** (each spec section → task):

| Spec section | Implemented by |
|---|---|
| `serve` / `pair` / `doctor` commands | Tasks 24–27 |
| `/stream/<id>` proxy with Range + URL cache | Tasks 6–8, 16 |
| `/api/search`, `/api/track`, `/api/album`, `/api/playlist` | Tasks 3–5, 15 |
| WebSocket transport, role negotiation, fan-out | Task 17 |
| Session state, queue, transport intent | Task 9 |
| Auto-advance with 5-skip cap | Task 10 |
| Alpine.js + plain-JS UI matching mockup | Tasks 20–23 |
| Single-token pairing via QR | Tasks 1, 12, 25 |
| Classified errors | Task 13, used by 16, 17, 18 |
| Token caching with 30-min safety margin | Task 6 |
| Bearer auth (header + `?t=` for `<audio>`) | Task 14 |
| Live integration test (gated, polite) | Task 28 |

**Type-consistency check:**
- `session.Track` is the canonical track type on the WS wire (Task 9). API responses use `apiTrack` (Task 15) which has the same shape with `cover_url` already expanded; the JSON encoding is identical, so controllers can pass API results back as `cmd` payloads' IDs without re-mapping.
- `Resolver` interface in `internal/transport` (Task 16) matches the `*media.Resolver` method (Task 7) exactly: `Resolve(ctx, trackID) (media.ResolvedTrack, error)`.
- `HubSink` interface (Task 18) matches `*Hub` methods (Task 17): `Broadcast(v any)`, `SendToPlayer(v any) bool`, `HasPlayer() bool`.
- `GatewayAPI` interface (Tasks 15, 18) matches `*gateway.Client` methods: `Search`, `SongGetData`, `Album`, `Playlist`.
- `TrackFetcher` interface in `internal/media` (Task 7) is satisfied by `fetcherAdapter` in `internal/transport/server.go` (Task 19) — strict layering preserved (media never imports gateway).
- Error kinds in `internal/transport/messages.go` (Task 13) match the spec's error-handling table exactly: `auth_expired`, `not_available`, `rate_limited`, `player_gone`, `region_locked`, `queue_exhausted`, `role_taken`, `internal`.

**Placeholder scan:** None remain. Every step has concrete code, exact commands, and expected output.

---
