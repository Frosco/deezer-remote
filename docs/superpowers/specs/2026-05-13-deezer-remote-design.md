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
2. Service calls `media.GetURL({tokens: [TRACK_TOKEN], license_token, formats: [MP3_320, MP3_128]})` against `https://media.deezer.com/v1/get_url` → CDN URL + size + chosen format. Spike measured ~20 h TTL; the in-memory cache (see Decisions) reuses entries while `remaining_ttl > 30 min`.
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
    src/                  Alpine.js view templates + plain JS modules (ws.js, stores.js, commands.js)
    dist/                 Final static assets, served via embed (no build step; src and dist may be the same in practice)
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

### Findings

**Date:** 2026-05-13
**Track:** `3135556` (Daft Punk — Harder, Better, Faster, Stronger; the plan's caption "Get Lucky" was misattributed)

- **MP3_320 returned:** ✓ — `format=MP3_320`, 9,059,264 bytes, ffprobe confirms 320 kbps / 44.1 kHz / stereo / 226 s. Output starts with MP3 frame sync (`0xFFE0`); plays end-to-end.
- **MP3_128 returned:** ✓ — `format=MP3_128`, 3,623,705 bytes, ffprobe confirms 128 kbps. Same track, distinct CDN host (`cdnt-stream.dzcdn.net` vs `f-cdnt-stream.dzcdn.net` for 320), distinct URL.
- **FLAC attempted:** not in spike (deferred).
- **Media URL TTL:** `exp - now ≈ 71,976 s ≈ 20 h`. Much longer than the design's "minutes" assumption. The `nbf` field in the response shape didn't surface in the live response, only `exp`. Implication: the in-memory `(track_id → URL, expiry)` cache the design hypothesised is safe with a generous safety margin (30 s is overkill; even 30 min is conservative).
- **Headers required beyond defaults:** none observed. `cookiejar` carrying `arl` + the server-assigned `sid` after the first call is sufficient for gw-light. The CDN GET succeeded with `http.DefaultClient`'s default headers — no `User-Agent` override, no `Origin`, no `Referer`, no auth header.
- **Region/availability behavior:** not exercised on this track (entitled, playable). The plan's error-handling path for `Data[0].Errors[...]` was wired into `media.GetURL` but only the happy path ran live.
- **Time from `--track` to `wrote <id>.mp3`:** sub-second end to end for the 320 path (decrypt alone: 584 ms; download dominant). MP3_128 was 306 ms decrypt.
- **Anomalies observed:**
  - Track `3135556` is "Harder, Better, Faster, Stronger", not "Get Lucky" as the spike plan's example caption claimed. Cosmetic plan-doc bug; the pipeline itself is correct.
  - The `media.deezer.com/v1/get_url` response did **not** include `nbf` in the live shape — only `exp`. Our wire decoder ignores fields beyond what it parses, so this is harmless, but a Phase-1 reader of the field should not assume `nbf` is present.
  - User-agent fingerprint: served from `http.DefaultClient` (Go's default `User-Agent: Go-http-client/1.1`) without rejection. Worth a comment that this may eventually be rate-limited or blocked by Deezer; not observed today.

**Verdict:** **GO** for Phase 1 design. The full pipeline (arl auth → gw-light CSRF → media.getUrl → CDN GET → Blowfish stride decrypt → playable MP3) works on Nils's account at MP3_320, and the 20-hour CDN URL TTL means `/stream/<id>` can comfortably cache URLs across multiple plays. No assumptions invalidated; one assumption (short TTL) is generously loosened.

## Error handling

| Source | Failure mode | What the user sees |
|---|---|---|
| Deezer auth | `arl` expired or revoked | Phone banner: "Deezer login expired — re-paste arl in config." Service refuses to start a new track; current track plays out. |
| Deezer content | `media.getUrl` returns no URL (region-locked, tier-locked, removed) | Phone toast: "Track not available." Skip to next in queue. Logged. |
| Local plumbing | CDN URL expired mid-stream | Service silently re-fetches `media.getUrl` and re-issues the upstream GET starting at the byte offset the browser is asking for. Browser doesn't notice. |
| Network | Player tab disconnects | Phone banner: "Desktop disconnected." Service holds session state for ~60 s; when player tab reconnects, it resumes from the saved position. After timeout, session pauses (but stays loaded). |
| Network | Phone disconnects | Service doesn't care; player keeps playing. Phone reconnect: state snapshot, catches up. |
| Protocol | Two browsers try to be player at once | Second one rejected with `error {kind: "role_taken"}`. Resolution in MVP: close the existing player tab and reload the new one — service drops the old WS within the heartbeat timeout. Forced-takeover UX is Phase 2. |

Errors over the wire use the classified-kind pattern: `{kind: "auth_expired"|"not_available"|"rate_limited"|"player_gone"|"region_locked"|"queue_exhausted"|"role_taken"|"internal", message: "..."}`. UI branches on `kind`, never on `message`.

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

## Decisions (resolved after spike, 2026-05-13)

- **Web UI framework: Alpine.js + plain JS modules.** Alpine drives the view layer via attributes (`x-data`, `x-show`, `x-for`, `@click`). The WebSocket client and reactive state stores live in plain JS modules under `web/src/` (`ws.js`, `stores.js`, `commands.js`) and Alpine reads from them. No Node toolchain; Alpine is loaded as a single `<script>` and assets ship via `embed.FS`. Rationale: declarative reactivity is what the UI needs, embed-friendly, no build pipeline. Migration backstop: if Phase 2 outgrows Alpine (deep routing, long-list virtualization), only the view layer migrates — WS/state modules stay.
- **Track-token caching: in-memory, 30 min safety margin.** Service maintains a process-local map `track_id → {cdn_url, expiry, format, sng_id, size}`. On `/stream/<id>`, reuse the cached entry if `remaining_ttl > 30 min`; otherwise refetch `media.getUrl`. Evict any entry on `4xx` from the CDN and retry once. Spike confirmed ~20 h TTL, so 30 min is generous headroom. Cache is in-memory only — not persisted across restarts.
- **Queue source: pre-load metadata, lazy-fetch media URLs.** When a controller plays an album or playlist, the service fetches the full track list metadata up-front via `/api/album/<id>` or `/api/playlist/<id>` and seeds the queue (albums/playlists are bounded; hundreds of tracks at worst). Media URLs are fetched lazily — only for the *current* track and the *next* track (pre-warm on play to keep gapless feel acceptable). Pre-warming further ahead is deferred.
- **Auto-advance on dead track: skip with toast, bounded at 5.** When `playback {ended: true}` arrives, the service advances the queue. If `media.getUrl` returns no URL (region-locked, removed, tier-locked) the service emits `error {kind: "not_available", message: "<title> — track unavailable"}` and advances again. After **5 consecutive skips** the service pauses the session and emits `error {kind: "queue_exhausted", message: "Couldn't find a playable track in this queue."}`. The skip counter resets on any successful play.

## Phase 2 candidates (informative, not in MVP)

- Library browsing from phone (loved albums, playlists, loved tracks).
- Queue editing from phone (insert, reorder, remove up-next).
- Windows SMTC integration so OS lock screen + media keys work.
- Multi-device player + transfer playback (the actual Connect feature). Requires non-trivial session state extension; design will need revisiting.
- Native Android client via TWA or Capacitor.
- Off-LAN access via Tailscale (zero code changes expected).
- FLAC / HiFi tier.

## TODO (no tracker until the project grows)

- [x] Spike: implement `cmd/spike/main.go` and write the findings note back into this doc.
- [x] Write the Phase 1 implementation plan (writing-plans skill).
- [x] Implement Phase 1 (this plan).

## Known follow-ups (post Phase 1, 2026-05-14)

Surfaced by the cross-cutting review after all 29 implementation tasks landed. None block shipping Phase 1; track here until the project grows enough to need a real tracker.

- [ ] **Layering exception for `internal/media/integration_test.go`.** The live integration test imports `internal/gateway` to set up the arl-auth → song.getData → media.getUrl pipeline against a stable public track. Production code in `internal/media` is gateway-free, so the strict-layering rule still holds for shipped binaries — but the test file is the one place the rule bends. Either explicitly document this exemption in the spec's "Code organisation" section, or move the test to a sibling `test/integration/` package so the dependency is unambiguous. Low priority — only matters if a future contributor reads the layering rule strictly.
- [ ] **Dead error-kind constants.** `transport.ErrKindPlayerGone` and `transport.ErrKindRegionLocked` are declared in `internal/transport/messages.go` but never emitted from any code path. They're part of the documented wire contract per the spec's "Error handling" table, so removal is wrong — but the spec's listed semantics (player disconnect heartbeat → `player_gone`; geo-block → `region_locked`) aren't wired up yet. Either wire them up (heartbeat-driven `player_gone` in `transport.Hub`, region-aware error mapping in `transport.api.writeGatewayError` once `gateway` classifies region errors), or trim the spec table to match the implemented surface.
- [ ] **Manual browser smoke test on the actual laptop.** The spec's "Manual smoke test plan" (cold pair, search → play, transport from phone, close-and-reopen player, `doctor` green) was deferred from Task 29 because it requires a real arl, a real LAN, a real phone. Run through it once on Nils's setup; record anything unexpected as a fresh follow-up here (don't patch in the same pass).
