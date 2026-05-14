---
title: Don't name methods Seek/Read/Write/Close unless the type implements that stdlib interface
date: 2026-05-14
category: conventions
module: internal/session
problem_type: convention
component: service_object
severity: low
applies_when:
  - Adding a method to a type that doesn't implement an io stdlib interface
  - Running go vet -all (or any CI that enables the stdmethods analyzer)
tags: [go, naming, stdmethods, io-seeker, vet, false-positive-trap]
---

# Don't name methods Seek/Read/Write/Close unless the type implements that stdlib interface

## Context

When implementing `internal/session/session.go` in Phase 1, the natural method name for "set the current playback position in milliseconds" was `Seek(positionMs int64)`. The plan literally specified `Seek`. `go build` was clean. `go test` was clean. But `go test -vet=all ./...` failed to even compile the package with:

```
internal/session/session.go:118:19: method Seek(positionMs int64) should have signature Seek(int64, int) (int64, error)
```

The `stdmethods` vet analyzer (enabled under `-vet=all`, off in the default `go test` vet subset) flags methods whose names match stdlib interface method signatures but whose own signatures don't match. `io.Seeker.Seek` is `Seek(offset int64, whence int) (int64, error)`. Our `Session.Seek(int64)` collides on the name and trips the check, even though `Session` is not meant to be an `io.Seeker`.

The fix was a one-character rename: `Seek` → `SeekTo`. But the trap is worth remembering: `go build` and the default `go test` vet subset both pass, so the problem only surfaces on stricter vet runs (linters, CI matrices that run `go vet -all`, IDE diagnostics that mirror it).

## Guidance

Reserve the following method names for types that actually implement the matching stdlib interface, signature and all:

| Name | Stdlib interface | Required signature |
|---|---|---|
| `Read` | `io.Reader` | `Read(p []byte) (n int, err error)` |
| `Write` | `io.Writer` | `Write(p []byte) (n int, err error)` |
| `Close` | `io.Closer` | `Close() error` |
| `Seek` | `io.Seeker` | `Seek(offset int64, whence int) (int64, error)` |
| `ReadAt` / `WriteAt` | `io.ReaderAt` / `io.WriterAt` | `(p []byte, off int64) (int, error)` |
| `ReadByte` / `WriteByte` | `io.ByteReader` / `io.ByteWriter` | `() (byte, error)` / `(byte) error` |
| `UnreadByte` / `UnreadRune` | `io.ByteScanner` / `io.RuneScanner` | `() error` |
| `Format` | `fmt.Formatter` | `Format(f fmt.State, c rune)` |
| `Scan` | `fmt.Scanner` | `Scan(state fmt.ScanState, verb rune) error` |
| `MarshalJSON` / `UnmarshalJSON` | `json.Marshaler` / `json.Unmarshaler` | `() ([]byte, error)` / `([]byte) error` |
| `MarshalXML` / `UnmarshalXML` | `xml.Marshaler` / `xml.Unmarshaler` | parameterised |

If the operation is a domain action that just happens to share a name (e.g., "seek to a playback position in a music session"), pick a name that doesn't collide. Good replacements that surfaced or are obvious:

- `Seek` → `SeekTo` (used here), `Jump`, `MoveTo`, `SetPosition`
- `Read` → `Fetch`, `Get`, `Load`
- `Write` → `Save`, `Persist`, `Store`
- `Close` → `Shutdown`, `Stop`, `Done` — but be aware `Done` collides with `context.Context.Done` if signatures diverge

## Why This Matters

- **Silent until strict CI.** Default `go vet` runs only a curated subset of analyzers; `stdmethods` isn't in it. `go build` doesn't catch it either. The first time you'll know is when a `-vet=all` job, a strict linter, or an IDE's diagnostic panel screams. By that point the name may already be in commits, plans, or downstream code.
- **`go test ./...` cached results can mask the problem.** A test run that succeeded before the `-vet=all` upgrade stays cached; you may need `go clean -testcache` (or modify a file in the package) before the vet failure surfaces under re-run.
- **The fix is cheap, the prevention is free.** Catching it in code review or at first commit costs nothing; finding it after the method has spread through callers is annoying but still bounded; finding it after release in a CI matrix you didn't know enabled `stdmethods` is the worst case.

## When to Apply

- New types in any Go package, especially state-machine-style objects (sessions, queues, pipelines) where "seek", "read", "write" feel like natural verbs.
- Pre-commit: before merging a PR that introduces a new public method, sanity-check the name against the stdlib interface table above. If the name matches, the signature must match too.
- CI: add `go vet -all ./...` (or equivalent — e.g. `staticcheck -checks=ST*,stdmethods`) to the project's verification gate so the trap surfaces in CI rather than locally.

## Examples

**Before** (`internal/session/session.go`, fails `go vet -all`):

```go
// Seek clamps position to [0, +inf) and stores it.
func (s *Session) Seek(positionMs int64) {
    s.mu.Lock()
    if positionMs < 0 {
        positionMs = 0
    }
    s.state.PositionMs = positionMs
    s.mu.Unlock()
}
```

**After** (passes `go vet -all`, domain meaning preserved):

```go
// SeekTo clamps position to [0, +inf) and stores it.
func (s *Session) SeekTo(positionMs int64) {
    s.mu.Lock()
    if positionMs < 0 {
        positionMs = 0
    }
    s.state.PositionMs = positionMs
    s.mu.Unlock()
}
```

Test names follow: `TestSession_Seek_ClampsAndStores` → `TestSession_SeekTo_ClampsAndStores`. Callers in `internal/transport/router.go` (the seek command handler) call `sess.SeekTo(p.PositionMs)`.

## Related

- Go documentation: `go doc cmd/vet stdmethods`
- Reference for `-vet=all` behaviour: https://pkg.go.dev/cmd/go#hdr-Testing_flags ("the default vet checks are a small subset of those in cmd/vet")
- The session SeekTo method that triggered this learning: `internal/session/session.go`
