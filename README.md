# deezer-remote

A single-user, on-LAN replacement for Deezer's discontinued *Connect* feature.

A Go binary runs on your laptop. The laptop browser becomes the **player**
(it does the actual audio playback). Your phone, on the same Wi-Fi network,
becomes the **controller** (queue, play/pause, skip, seek, volume). The two
talk over WebSocket.

## Caveats — read before you start

- **Not affiliated with Deezer.** This project is not endorsed, sponsored, or
  supported by Deezer. It uses Deezer's private `gw-light` API and CDN; both
  can change at any time and break this software.
- **You need your own Deezer account.** You sign in by copying *your own*
  `arl` cookie into a local config file. Do not share that cookie — it grants
  full access to your account. Do not paste someone else's.
- **Personal use only.** Audio is decrypted in-process so the browser can play
  it. Do not redistribute, persist, or re-upload the decrypted stream. Treat
  this like any other personal playback client.
- **Single-user, LAN-only.** There is no multi-user mode and no public-internet
  story. Exposing the port to the internet would expose your Deezer session.
- **Phase 1.** The feature set is intentionally small: queue, play/pause, skip
  forward/back, seek, volume, search, browse album/playlist. No gapless, no
  offline cache, no cross-device handoff. Design spec:
  [`docs/superpowers/specs/2026-05-13-deezer-remote-design.md`](docs/superpowers/specs/2026-05-13-deezer-remote-design.md).

## Requirements

- A laptop running **Linux** (primary) or **Windows** (supported but less
  exercised). macOS is not tested.
- **Go 1.25** or newer (only required to build from source).
- An active **Deezer** account (Premium recommended — the API will refuse
  high-quality streams on free accounts).
- The laptop and the phone must be on the **same Wi-Fi network**, and the
  network must permit traffic between clients (most home networks do; many
  guest / "AP isolation" networks do not).
- Port **8080** free on the laptop (configurable with `--port`).

## Install

```bash
git clone https://github.com/niref/deezer-remote.git
cd deezer-remote
go build -o deezer-remote ./cmd/deezer-remote
```

The binary is self-contained: the web UI is embedded with `embed.FS`. Move
`./deezer-remote` anywhere on your `$PATH` if you like.

## Configure

### 1. Find your `arl` cookie

The `arl` cookie is what proves to Deezer that the requests are coming from
your logged-in session. To extract it:

1. Sign in to <https://www.deezer.com> in your browser.
2. Open DevTools → **Application** (Chrome) or **Storage** (Firefox) →
   **Cookies** → `https://www.deezer.com`.
3. Find the cookie named `arl` and copy its **Value** (a long hex string).

Keep it secret. Anyone with this value can act as you on Deezer.

### 2. Write the config file

Create `config.toml` at:

- **Linux:** `~/.config/deezer-remote/config.toml`
- **Windows:** `%APPDATA%\deezer-remote\config.toml`

```toml
arl = "paste-your-arl-cookie-here"
```

On **Linux**, the file must be mode `0600` or the program will refuse to
read it:

```bash
chmod 0600 ~/.config/deezer-remote/config.toml
```

The `bearer_token` line will be added automatically the first time you run
`pair` or `serve` — do not write it yourself.

## First-time pairing

```bash
./deezer-remote pair
```

This:

1. Generates a 32-byte bearer token (stored in `config.toml`).
2. Prints a **player URL** for the laptop (e.g. `http://localhost:8080/?t=...`).
3. Prints one or more **phone URLs** for each detected LAN interface, each
   with a QR code in the terminal.

Scan the phone QR with your phone's camera. The bearer token is in the URL,
so the phone is paired the moment you open the link.

If you ever leak the token or want to un-pair a device, rotate it:

```bash
./deezer-remote pair --reset
```

All previously paired devices will need to re-scan.

## Daily use

```bash
./deezer-remote serve
```

This starts the HTTP + WebSocket server on `0.0.0.0:8080`, authenticates
against Deezer with your `arl`, and reprints the player + phone URLs. Open:

- The **player URL** on the laptop (this tab is the audio output — keep it
  open, keep its volume up).
- The **phone URL** on your phone (this is the remote).

Search and queue tracks from the phone; the laptop plays them.

Stop with `Ctrl-C`.

## Commands

| Command | Purpose |
|---|---|
| `deezer-remote serve` | Start the service. Add `--port 9000` to change the port, `--bind 127.0.0.1` to restrict to localhost. |
| `deezer-remote pair` | Print player + phone URLs and the QR for the current bearer token. |
| `deezer-remote pair --reset` | Rotate the bearer token, then print the new URLs. Invalidates all previously paired phones. |
| `deezer-remote doctor` | Run end-to-end self-checks: config readable, `arl` authenticates, port can bind, LAN reachable, a real track streams. Use this first when something is broken. |

## Troubleshooting

**`config has permission 0644, expected 0600`** — Linux refuses world- or
group-readable config. Run `chmod 0600 ~/.config/deezer-remote/config.toml`.

**`authenticate with Deezer (arl)`** — Your `arl` is empty, malformed, or
expired. Re-copy it from a fresh browser session.

**Phone can't reach the laptop** — Confirm both are on the same SSID, and
that the network isn't a guest network with client isolation. Many corporate
and café Wi-Fi networks block client-to-client traffic. Run
`deezer-remote doctor` — it will probe the LAN addresses.

**Audio doesn't start in the laptop tab** — Browsers block autoplay until
you interact with the page. Click anywhere in the player tab once.

**`Invalid CSRF token`** in logs — Transient; the gateway client refreshes
and retries automatically. If it persists, your `arl` is probably stale.

**Stream stutters or stops mid-track** — Deezer's signed CDN URLs expire
(~20 h). The cache evicts old entries, but a CDN 4xx mid-stream is retried
once. If it happens repeatedly, restart `serve`.

## How it works (one paragraph)

The Go service speaks Deezer's `gw-light` JSON-RPC API (auth, search,
song/album/playlist lookup) and its `media.getUrl` endpoint to obtain
short-lived CDN URLs. Audio is delivered encrypted (Blowfish-CBC on every
third 2048-byte block; key derived from the track ID); the service decrypts
on the fly and serves the result over a Range-aware `/stream/<track-id>`
endpoint that the player tab's `<audio>` element pulls from. Authoritative
session state (queue, current track, position, volume) lives in the Go
process; the player tab pushes actual playback time over WebSocket; the
controllers are pure renderers + command emitters. Wire protocol, error
classification, and architectural rules are in the design spec linked above.

## Development

Internal architecture notes, the test corpus layout, and the known
gotchas are in [`CLAUDE.md`](CLAUDE.md). Run the test suite with:

```bash
go test ./...                # default; fast
go test -vet=all ./...       # include stdmethods analyzer (recommended in CI)
```

Live tests against the real Deezer API are opt-in:

```bash
DEEZER_INTEGRATION=1 go test ./internal/media -run TestIntegration_GetMediaURL
```

These require a working `arl` in `config.toml` and are Linux-only.

## License

See [LICENSE](LICENSE).
