# deezer-remote — design

**Status:** brainstorming → spec. Awaiting Nils review before plan writing.
**Date:** 2026-05-13
**Companion docs:** `docs/idea.md` (initial sketch), `docs/deezer_remote_app_mockup.html` (UI mockup).

## Goal

Replace the discontinued Deezer Connect feature for one user. From a phone, search for music and have it play on a Windows laptop's speakers; keep transport (play/pause/seek/volume) live and synchronised between phone and laptop. Personal, single-account, on-LAN.

## Non-goals (MVP)

The bright line that keeps this finishable. Anything below is **deferred**, not "later in the same milestone":

- Multi-device players, device picker, "transfer playback to X" UX. *This is the natural Phase 2.*
- Native Android app. Phone uses the same web UI in its browser.
- Off-LAN reach (Tailscale, relay server, NAT traversal). Free to bolt on later — design doesn't change.
- Queue editing from phone (insert/reorder/remove). Read-only "up next" is allowed.
- Library browsing from phone (loved albums/tracks/playlists screens). Search is the only entry point.
- Lyrics, history, cross-fade, gapless, FLAC, HiFi-tier handling.
- TLS, per-device tokens, token rotation, kick-paired-device UX.
- System tray app / native window (Wails/Tauri).
- mDNS / auto-discovery. QR-code-on-startup only.
- Windows SMTC / MediaSession (now-playing on lock screen, media-key control).
- Multiple Deezer accounts on one binary.

## Architecture

```
                Deezer (gw-light + CDN)
                          ▲
                          │ arl auth, song.getData, media.getUrl, encrypted chunks
                          │
                ┌─────────┴─────────┐
                │  deezer-remote    │   Go binary on the Windows laptop.
                │   (Go service)    │   - Session state: current track, queue,
                │                   │     play/pause intent, volume target.
                │                   │   - Decrypts streams; serves /stream/<id>
                │                   │     as plain MP3 to the player tab.
                │                   │   - Single HTTP + WS server on :8080.
                └────┬─────────┬────┘
                     │  WS     │  WS
                     │         │
              ┌──────▼──┐   ┌──▼────────┐
              │ Player  │   │ Controller │ Same web bundle; role chosen on connect.
              │  tab    │   │   tab(s)   │ Multiple controllers OK; one player at a time.
              │ (laptop │   │  (phone    │
              │ browser)│   │  browser)  │
              └─────────┘   └────────────┘
                <audio> plays   No audio; sends commands, renders state.
                /stream/<id>
```

### Roles and authority

- **Go service** is authoritative for *session state*: current track ID, queue, play/pause intent, volume target. Survives browser-tab reloads. Knows when the player tab is alive (WS ping/pong heartbeat).
- **Player tab** (laptop browser) is authoritative for *actual playback time*. The `<audio>` element's `currentTime` is the only real source. Player tab pushes `playback` messages over WS every ~250 ms; service fans them out to controllers. The player tab is also the only thing that actually pauses, seeks, etc. — the service tells it to.
- **Controller tabs** (phone browsers) are pure renderers + command emitters. They never touch audio.

### MVP user flow

1. User runs `deezer-remote serve` on the laptop. Terminal prints the player URL (clickable) and the phone URL (QR code).
2. User clicks the player URL on the laptop → laptop browser opens a tab in player role.
3. User scans the QR with their phone camera → phone browser opens the same web UI in controller role.
4. User types into search box on phone, taps a result.
5. Phone WS → service: `cmd {kind: "play_track", payload: {track_id}}`.
6. Service: updates session state, sends `do {kind: "load", payload: {track_id}}` to player tab.
7. Player tab issues `GET /stream/<track_id>` against the service. Service fetches encrypted bytes from Deezer CDN, decrypts on the fly, streams back as plain MP3. Player tab's `<audio src="/stream/<id>">` plays it.
8. Player tab pushes `playback {position_ms, paused, ended}` every ~250 ms. Service relays state snapshots to phone. Phone UI updates.
9. Phone transport commands (pause/seek/skip/volume) → service → player tab.

## Wire protocol

### HTTP endpoints (Go service on `:8080`)

- `GET /` — single SPA bundle; layout adapts phone vs laptop by viewport.
- `GET /stream/<track_id>` — decrypted MP3 bytes, Range-aware. Used by the player tab only. Internally: `song.getData` → `media.getUrl` → fetch encrypted bytes from Deezer CDN → Blowfish-decrypt-as-you-stream → out to browser. `Content-Length` from the size field of `media.getUrl`.
- `GET /api/search?q=...` — proxy over gw-light search. Returns JSON shaped for the phone UI.
- `GET /api/track/<id>`, `/api/album/<id>`, `/api/playlist/<id>` — metadata for detail views.
- `WS /ws?t=<token>` — live channel; both roles connect here.

All `/api/*` and `/stream/*` require `Authorization: Bearer <token>` (phone) or query token (player tab's `<audio>` element can't set headers, so `/stream` accepts `?t=<token>` as a fallback).

### WebSocket messages

JSON, `{"type": "...", ...}`. Authentication via `?t=<token>` on connect; rejected connections close immediately.

| From | Type | Fields | When |
|---|---|---|---|
| Tab → service | `hello` | `role: "player"\|"controller"` | First message on connect |
| Player → service | `playback` | `position_ms, paused, ended` | Every ~250 ms while playing; immediately on state-change events |
| Controller → service | `cmd` | `kind, payload` (kinds: `play_track`, `play`, `pause`, `next`, `prev`, `seek`, `set_volume`) | User taps a control |
| Service → player | `do` | `kind, payload` (kinds: `load`, `play`, `pause`, `seek`, `set_volume`) | Service translates a controller cmd into a player instruction |
| Service → all | `state` | `current_track, queue, position_ms, paused, volume` | Full snapshot on connect; deltas on change |
| Service → tab | `error` | `kind, message` | Player gone, stream failure, auth, region lock, etc. |

### The streaming proxy (`/stream/<id>`)

For each request:
1. Service calls `gateway.SongGetData(track_id)` → `TRACK_TOKEN`, `SNG_ID`.
2. Service calls `media.GetURL({tokens: [TRACK_TOKEN], license_token, formats: [MP3_320, MP3_128]})` against `https://media.deezer.com/v1/get_url` → CDN URL + size + chosen format. URL has a short TTL (minutes).
3. Service issues a Range-aware GET to that CDN URL.
4. Bytes are decrypted on the fly: Blowfish-CBC on every 6144-th 2048-byte block; passthrough otherwise. Key derived from `md5(SNG_ID)` XOR'd with a known 16-char constant. IV is the fixed `0x0001020304050607`.
5. Range requests from the browser map to byte ranges in the encrypted file; alignment to 2048-byte blocks handled inside the proxy.
6. If the CDN URL expires mid-stream (HTTP 403 or similar), the proxy silently re-fetches `media.getUrl` and resumes from the byte offset the browser is currently asking for.

The **spike** (Section "Spike") validates steps 1–4 end-to-end before we commit to building the rest.

## Pairing, discovery, auth

### Two auth surfaces — don't confuse them

- **Deezer auth** — the `arl` cookie. Stored in `os.UserConfigDir()/deezer-remote/config.toml`. Same pattern as deezer-tools. On Linux: enforce `0600` mode; refuse to start otherwise. On Windows: skip the mode check and rely on `%APPDATA%\Roaming\deezer-remote\` being user-private by default ACLs. Documented behavior: "On Windows, security relies on `%APPDATA%` being user-private; don't move the file elsewhere."
- **Phone↔laptop auth** — a 32-byte bearer token the service generates on first run and persists in the same config file. Independent of the arl.

### Pairing flow (first run)

1. `deezer-remote serve` starts. If no bearer token in config, generates one (32 random bytes, base64url).
2. Service enumerates network interfaces, picks the private LAN address (`192.168.x.y` / `10.x.y.z` / `172.16-31.x.y`). If ambiguous, prints all candidates. Override via `--bind`.
3. Terminal prints:
   - **Player URL:** `http://localhost:8080/?t=<token>` — clickable link, open in laptop browser.
   - **Phone URL** as a **QR code** rendered in the terminal: `http://192.168.x.y:8080/?t=<token>`. Scan with phone camera → opens in phone browser.
4. Each page on first load strips `?t=` from the URL, stores the token in `localStorage`, then uses it as `?t=` on the WebSocket connect and `Authorization: Bearer <token>` on every `/api/*` request. `/stream/<id>` requests (from the `<audio>` tag) use `?t=` because audio tags can't set headers.
5. Subsequent visits: bookmarked URL has no token; page reads it from localStorage. Works forever.

### Token loss recovery

`deezer-remote pair` reprints the QR + URLs (no regeneration). `deezer-remote pair --reset` rotates the token and reprints — invalidates any previously-paired device. No revocation per device in MVP.

### Threat model

For personal home use, equivalent to the threat model of `http://192.168.1.1` (your router admin page). Anyone on your WiFi who guesses your IP and the 32-byte token can use the service — effectively zero risk. Plain HTTP on LAN (no TLS); anyone sniffing packets sees the bearer token. **Not in scope:** TLS, per-device tokens, kick-paired-device.

### Windows first-run checklist

The binary works on Windows only after the following operational setup is in place. The `doctor` subcommand checks each of these.

1. **Windows Firewall.** First time the binary binds `0.0.0.0:8080` and the phone connects from a non-loopback IP, Windows pops up "Allow `deezer-remote.exe` on Private / Public networks?" — tick **Private**, click Allow. Persistent thereafter.
2. **Network profile = Private**, not Public (Windows Settings → Network).
3. **No AP isolation on the router.** If the phone can `ping <laptop-ip>`, fine.
4. **DHCP reservation on the router** (recommended) so the laptop's IP doesn't drift. Alternative: re-pair via `deezer-remote pair` after IP changes.
5. **Antivirus suites** with their own firewalls layered on top of Windows Defender may also need an allow rule.

### `deezer-remote doctor` (in MVP)

Validates:
- arl is present and reachable (calls `getUserData`, prints account email).
- Bearer token exists.
- Binary can bind the configured port.
- Each enumerated LAN IP can accept a TCP connection from itself within 30 s (probes localhost + each bind address).
- A test track ID (configurable, default a stable public track) is fetchable end-to-end through `/stream/<id>` (range-bounded to first 64 KB).

Exits non-zero on any failure with a one-line hint per failure. Used as: install → configure arl → `doctor` → if green, `serve`.

## Code organisation

Single repo `deezer-remote`. `deezer-tools` stays untouched. If overlap surfaces after the MVP stabilises, that's the trigger to extract a shared gateway package — not before.

```
deezer-remote/
  cmd/
    deezer-remote/        Cobra wiring: serve, pair, doctor
    spike/                Throwaway: cmd/spike/main.go (Section "Spike")
  internal/
    config/               Loads arl + bearer token from os.UserConfigDir()
    gateway/              gw-light client: auth, CSRF, Call, classified errors
    media/                song.getData, media.getUrl, the decryption streaming proxy
    session/              Authoritative session state, command application, role conflict
    transport/            HTTP + WebSocket server, role negotiation, fan-out
    web/                  embed.FS for the built SPA bundle
  web/
    src/                  HTML/CSS/JS source — framework decided after the spike (see Open questions)
    dist/                 Built static assets, served via embed
  docs/
    superpowers/specs/    This document and successors
    idea.md               Original sketch
    deezer_remote_app_mockup.html
```

Strict layering: `cmd → session → media → gateway`. `transport` depends on `session`. `media`, `session`, `transport` never cross-import. The gateway primitive is just `Call(ctx, method, params) -> json.RawMessage, error` plus classified error types; everything else builds on top.

The gateway re-implements the *pattern* of deezer-tools' gateway (cookie jar, CSRF refresh-and-retry, classified errors) rather than copying the code. The methods we'll need are a subset of deezer-tools' surface plus the streaming additions; ~200-300 lines.

## Spike (one-day deliverable, gates the rest)

Before any of the above is built, prove the streaming pipeline works on Nils's account.

### Deliverable

A small Go program at `cmd/spike/main.go` inside `deezer-remote`. Run with a track ID:

```
$ go run ./cmd/spike --track 3135556
playing: Daft Punk - Get Lucky (MP3_320)
got CDN url (TTL 1m48s), file size 8.4 MB
decrypted in 1.2s
wrote ./3135556.mp3
$ ffplay 3135556.mp3   # or just open it
```

### What it does

The full pipeline, no shortcuts:

1. Read `arl` from `os.UserConfigDir()/deezer-remote/config.toml` (Linux: enforce `0600`).
2. gw-light `deezer.getUserData` → `license_token`, CSRF.
3. gw-light `song.getData(track_id)` → `TRACK_TOKEN`, `SNG_ID`, `MD5_ORIGIN`, `MEDIA_VERSION`.
4. POST `media.deezer.com/v1/get_url` with `{track_tokens: [TRACK_TOKEN], license_token, formats: [{cipher: "BF_CBC_STRIPE", format: "MP3_320"}, {cipher: "BF_CBC_STRIPE", format: "MP3_128"}]}` → CDN URL + size + format.
5. HTTP GET the CDN URL → encrypted bytes.
6. Blowfish-CBC decrypt every 6144-th 2048-byte block. Key derived from `md5(SNG_ID)` XOR'd with the known 16-char constant; IV `0x0001020304050607`. Math copied from deemix / d-fi rather than re-derived.
7. Write decrypted bytes to `./<track_id>.mp3`.

### Success criteria

The output file plays in any audio player. That single test proves: arl auth works, `media.getUrl` is reachable and honours the account tier, key derivation is correct, block alignment is correct, the CDN URL works without additional auth headers.

### Questions the spike answers (design depends on these)

- Does arl-only auth get us **MP3_320** (paid tier) or only **MP3_128**?
- What's the **TTL** on a media URL? (Drives whether `/stream/<id>` can cache it or must fetch fresh per request.)
- Any **User-Agent / header** requirements? (deemix sets specific ones; we'll find out which matter.)
- What does `media.getUrl` return for a **track unavailable in your region**? (Drives error handling.)

### Not in the spike

No Range requests, no HTTP server, no WebSocket, no UI, no pairing, no search, no queue, no graceful errors beyond a clear message. The point is to de-risk steps 1–7 of the audio pipeline. Nothing else.

### On failure

The spike writes both the encrypted bytes (`<id>.enc`) and the partially-decrypted result (`<id>.mp3.partial`) so we can diff against deemix output for the same track ID. If we can't make it work in a day, re-open the player-model decision — Option B (wrap Deezer's web player) is back on the table, or "don't build this at all".

### Output is a findings note

Appended to this design doc: "Spike on YYYY-MM-DD, track `<id>`: MP3_320 ✓ / FLAC ? / media URL TTL = X / headers required: Y. Notes: ...". That note unblocks the rest of the design.

## Error handling

| Source | Failure mode | What the user sees |
|---|---|---|
| Deezer auth | `arl` expired or revoked | Phone banner: "Deezer login expired — re-paste arl in config." Service refuses to start a new track; current track plays out. |
| Deezer content | `media.getUrl` returns no URL (region-locked, tier-locked, removed) | Phone toast: "Track not available." Skip to next in queue. Logged. |
| Local plumbing | CDN URL expired mid-stream | Service silently re-fetches `media.getUrl` and re-issues the upstream GET starting at the byte offset the browser is asking for. Browser doesn't notice. |
| Network | Player tab disconnects | Phone banner: "Desktop disconnected." Service holds session state for ~60 s; when player tab reconnects, it resumes from the saved position. After timeout, session pauses (but stays loaded). |
| Network | Phone disconnects | Service doesn't care; player keeps playing. Phone reconnect: state snapshot, catches up. |
| Protocol | Two browsers try to be player at once | Second one rejected with `error {kind: "role_taken"}`. Resolution in MVP: close the existing player tab and reload the new one — service drops the old WS within the heartbeat timeout. Forced-takeover UX is Phase 2. |

Errors over the wire use the classified-kind pattern: `{kind: "auth_expired"|"not_available"|"rate_limited"|"player_gone"|"region_locked"|"internal", message: "..."}`. UI branches on `kind`, never on `message`.

## Testing

### Unit tests (Go)

- `gateway`: faked HTTP transport, parametric tables for CSRF refresh, error classification, response shapes. Mirror deezer-tools' approach.
- `media`: known-good encrypted/decrypted byte pairs (recorded once via the spike against a free-tier track; checked in as `testdata/`). Verifies key derivation and 6144-block alignment.
- `session`: state machine tests — queue advance, command application order, role conflict, player-gone timeout.
- `transport`: WebSocket frame round-tripping, role negotiation, command routing, auth rejection.

### One live integration test

Linux-only, gated like deezer-tools. `DEEZER_INTEGRATION=1 go test ./internal/media -run TestIntegration_GetMediaURL` reads the real `arl`, does `getUserData → song.getData → media.getUrl` against a stable known-public track, verifies the returned URL is reachable (HEAD only — does not download bytes, to be polite to the CDN). Catches "Deezer changed something" before users do.

### Manual smoke test plan

Documented per release:

- Pair from cold (QR scan, token in URL).
- Search "Daft Punk" on phone → tap track → audio plays on laptop within 2 s.
- Pause / resume / seek to 50% / skip / change volume — all from phone, reflected on laptop.
- Close player tab → phone banner appears. Reopen player tab → resumes within 5 s.
- `deezer-remote doctor` reports green.

### Not tested

Browser audio playback automation (Playwright + `<audio>` is flaky and slow). FLAC decoding (out of scope). Multi-controller concurrency (one phone in MVP). Windows-specific behaviour is covered by manual smoke on Windows only.

## Open questions / decisions to revisit

- **Web UI framework.** Plain HTML+JS, htmx + a sprinkle of JS, Alpine.js, or a small SPA in Svelte / Preact / Vue? Decided when we start the UI work, after the spike. Constraints: must look and feel like the mockup (dark, modern, mobile-friendly), must embed cleanly into `embed.FS`, no Node-toolchain explosion.
- **Track-token caching.** Should the service cache the `(track_id → CDN URL, expiry)` mapping in memory and reuse within the TTL, or fetch fresh per `/stream/<id>` request? Spike will tell us how short the TTL is. Default plan: cache with 30 s safety margin.
- **Queue source.** Search results can be a track, an album, or a playlist; tapping any of them sends a single `cmd` to the service that resolves to "play this thing now". For a track → queue is just `[track]`. For an album/playlist → service fetches the contents via `/api/album/<id>` or `/api/playlist/<id>` and seeds the queue. Open: pre-load all track metadata up front, or fetch the next track lazily? Default plan: pre-load on play, since albums/playlists are small (hundreds of tracks at worst).
- **Auto-advance.** When `playback {ended: true}` arrives from the player tab, the service advances the queue. Edge case: what if the next track is region-locked and `media.getUrl` returns no URL? Skip with toast, advance again. Bound the skip loop (max 5 skips in a row before giving up).

## Phase 2 candidates (informative, not in MVP)

- Library browsing from phone (loved albums, playlists, loved tracks).
- Queue editing from phone (insert, reorder, remove up-next).
- Windows SMTC integration so OS lock screen + media keys work.
- Multi-device player + transfer playback (the actual Connect feature). Requires non-trivial session state extension; design will need revisiting.
- Native Android client via TWA or Capacitor.
- Off-LAN access via Tailscale (zero code changes expected).
- FLAC / HiFi tier.

## TODO (no tracker until the project grows)

- [ ] Spike: implement `cmd/spike/main.go` and write the findings note back into this doc.
- [ ] After spike: write the implementation plan (writing-plans skill).
