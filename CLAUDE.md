# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`deezer-remote` is a single-user, on-LAN replacement for the discontinued Deezer Connect feature: a Go binary runs on a laptop, plays decrypted MP3 audio in a "player" browser tab, and accepts transport commands from "controller" browser tabs (the user's phone). Phase 1 is complete and on `main`. The locked design lives at `docs/superpowers/specs/2026-05-13-deezer-remote-design.md` — read it before non-trivial changes; it is authoritative for scope, wire protocol, and resolved decisions.

## Common commands

```bash
go build ./...                            # build everything; produces ./deezer-remote when run from cmd/deezer-remote
go test ./...                             # default vet subset + tests
go test -vet=all ./...                    # full vet (includes stdmethods — see "Traps")
go test ./internal/media -run TestX       # single test
go mod tidy                               # after touching go.mod
DEEZER_INTEGRATION=1 go test ./internal/media -run TestIntegration_GetMediaURL
                                          # live test against Deezer; reads real arl from config; Linux-only
go run ./cmd/deezer-remote serve          # start service (needs config/arl); also: `pair`, `pair --reset`, `doctor`
```

There is no `cmd/spike/` anymore — the spike was merged in commit `5dc1a68` after validating the streaming pipeline on track `3135556`. Findings are appended to the design spec.

## Architecture

Single binary, three roles wired together by `cmd/deezer-remote`:

```
Deezer gw-light + CDN  ──►  internal/gateway   (auth, CSRF, classified errors, Call)
                                  ▲
                                  │
internal/media   ◄────────────────┘   (track URL resolution + cache, Blowfish-stride decrypt, Range-aware proxy)
   ▲
   │
internal/session    (authoritative State: queue, current, position, paused, volume; skip-dead cap = 5)
   ▲
   │
internal/transport  (http.Server + WS Hub; routes controller cmds → session/player; serves embedded SPA)
```

**Strict layering — enforce it.** `cmd → transport → session → media → gateway`. Layers below must not import layers above. `session` does not import `media` or `gateway`; `media` does not import `gateway` (the transport-layer `fetcherAdapter` bridges them at runtime). One documented exception: `internal/media/integration_test.go` imports `internal/gateway` to drive the live test — production code in `media` stays gateway-free.

### The player/controller protocol

- `/ws` accepts both roles; first message must be `{type:"hello", role:"player"|"controller"}`. At most one player; many controllers; a second player gets `error{kind:"role_taken"}` and is closed.
- Player owns *actual playback time* (pushes `{type:"playback", position_ms, paused, ended}` every ~250 ms); service owns *session state*; controllers are pure renderers + command emitters.
- The transport `Hub` and `CmdRouter` form a cycle: `NewRouter(...nil...)` then `router.SetHub(hub)`. Don't try to construct them in one shot.
- Wire constants (`ErrKind*`, `Cmd*`, `Do*`, `Role*`) live in `internal/transport/messages.go`. UI branches on `kind`, never on `message`. `ErrKindPlayerGone` and `ErrKindRegionLocked` are declared but not emitted yet — see post-Phase-1 follow-ups in the spec.

### Two unrelated auth surfaces

1. **Deezer auth** — the `arl` cookie, loaded from `os.UserConfigDir()/deezer-remote/config.toml`. On Linux the file must be mode `0600` or `config.LoadFromPath` refuses. On Windows the check is skipped and security relies on `%APPDATA%` ACLs.
2. **Phone↔laptop auth** — a 32-byte bearer token also in `config.toml`. Generated on first `serve`/`pair`; rotated by `pair --reset`. Enforced by `transport.RequireToken` middleware via either `Authorization: Bearer …` or `?t=…` (the `<audio>` tag can't set headers, so `/stream/<id>?t=…` is the fallback). Compare is constant-time.

### Streaming proxy

`/stream/{id}` resolves the track (cache → `song.getData` → `media.getUrl`), then streams Range-aware decrypted MP3 bytes. Decryption is Blowfish-CBC every 6144-th 2048-byte block; key is `md5(SNG_ID)` XOR'd with the constant `g4el58wc0zvf9na1`; IV is fixed `0x0001020304050607`. Block alignment is handled inside `media.NewRangeStream`; the test corpus in `internal/media/testdata` pins the encrypted/decrypted byte pairs. CDN URLs have a ~20 h TTL; the in-memory cache reuses entries while `remaining_ttl > 30 min`. If the CDN 4xx's mid-stream, evict and retry once.

## Traps that have bitten this repo

- **`stdmethods` vet analyzer.** `go build` and the default `go test` vet subset pass even when a method name collides with an `io` interface; `go test -vet=all ./...` does not. We hit this with `Session.Seek(int64)` → renamed to `SeekTo`. Reserve `Read`/`Write`/`Close`/`Seek`/`ReadAt`/`MarshalJSON`/… for types that actually implement the matching stdlib interface, signature and all. Convention doc: `docs/solutions/conventions/go-method-names-vs-stdlib-interfaces-2026-05-14.md`.
- **Cookie jar is load-bearing for the gateway.** `gw-light` binds the CSRF `api_token` to the server-set `sid` cookie. Replacing `http.Client` without preserving `cookiejar.Jar` will break every authenticated call with `Invalid CSRF token`. CSRF refresh-and-retry happens automatically inside `gateway.Client`.
- **Range alignment.** `media.NewRangeStream` aligns the request down to a 2048-byte block boundary, then trims leading slack. If a future caller bypasses this and decrypts mis-aligned bytes, the output will be silently corrupt — the every-3rd-block stride pattern depends on the offset.
- **`media.getUrl` response shape.** The spike observed `exp` but not `nbf` in the live response; don't assume `nbf` is present in any reader.
- **RFC1918 ≠ phone-reachable.** `internal/network.LANAddrs` filters by both RFC1918 membership *and* interface-name prefix; it skips `docker*`, `br-*`, `veth*`, `virbr*`, `vmnet*`, `vboxnet*`, `wsl*` because those bridges hand out private IPv4 addresses but live host-internal. Bare `br0` (user-configured bridge) and VPN-style interfaces (`tun*`, `tap*`, `wg*`, `tailscale*`) are deliberately not filtered — they can legitimately be the route to the phone. The companion gotcha: even with the right address advertised, Linux `firewalld` in the default `public` zone silently drops inbound 8080. `doctor`'s "LAN reachable from self" check can't catch this — `probeTCP` originates on the same host, and firewalld doesn't filter loopback / same-host traffic. README's "Phone can't reach the laptop" entry has the per-SSID zone recipe.

## Where work is tracked

- `docs/superpowers/specs/` — locked design specs (one per major phase).
- `docs/superpowers/plans/` — implementation plans (one per phase). Phase 1's plan is large (~80k tokens); read sections via `smart_outline` or by `offset`/`limit` rather than whole-file Reads.
- `docs/solutions/` — compounded learnings (conventions, debugging recipes). Searched proactively by the `ce-learnings-researcher` agent before brainstorming/plans/debugging.
- The spec's "Known follow-ups (post Phase 1)" section is the de facto issue tracker until the project grows.

## Conventions worth knowing

- Errors over the wire use the classified-kind pattern: `{kind, message}`. New error kinds go in `messages.go`; UI must branch on `kind`.
- Gateway methods return sentinel errors (`ErrAuthFailed`, `ErrNotFound`, `ErrRateLimited`, `ErrCSRFExpired`, `ErrServerError`, `ErrUnknown`); higher layers map them via `errors.Is` (see `transport.writeGatewayError`).
- The SPA is shipped via `embed.FS` from `internal/web/dist`. There is no Node build step; `web/src/` and `web/dist/` may be the same files in practice. Alpine.js is loaded as a single `<script>` from `vendor/`.
- No backwards-compatibility shims and no feature flags unless explicitly requested. YAGNI bias is strong in this repo.
