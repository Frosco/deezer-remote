---
title: Alpine 3 bootstrap module must precede the Alpine script tag in index.html
date: 2026-05-14
category: runtime-errors
module: internal/web
problem_type: runtime_error
component: frontend_stimulus
symptoms:
  - "ReferenceError: appRoot is not defined when Alpine evaluates x-data=appRoot"
  - "TypeError: Cannot read properties of undefined reading store.session and store.ui"
  - "27 identical-family console errors per page load; SPA renders as a black page with no controls"
  - "Affects both /?role=player and /?role=controller after deezer-remote serve"
root_cause: async_timing
resolution_type: code_fix
severity: critical
tags: [alpine-js, script-loading, defer, es-modules, microtask, alpine-init, embed-fs, spa]
---

# Alpine 3 bootstrap module must precede the Alpine script tag in index.html

## Problem

The deezer-remote SPA rendered as a black page on first load because Alpine's `alpine:init` event fired before `app.js` had a chance to register its listener, so neither the `appRoot` component nor the `$store.session`/`$store.ui` stores existed when Alpine processed the DOM.

## Symptoms

- Pure dark page on `http://localhost:<port>/?t=<token>&role=player` — neither `.player-shell` nor `.controller-shell` rendered.
- 27 console errors in three identical families:
  - `ReferenceError: appRoot is not defined`
  - `ReferenceError: role is not defined` (also `search`, `seekPct`, `fmt`)
  - `TypeError: Cannot read properties of undefined (reading 'init' | 'current' | 'connected' | 'paused' | 'volume' | 'errors')` against `$store.session` and `$store.ui`.
- All static assets returned HTTP 200 (alpine.min.js, app.js, all six JS modules, styles.css, Tabler icons).
- Mid-failure `browser_evaluate` snapshot proved Alpine had started but no registration had run:

  ```
  hasAlpine: true, alpineVersion: "3.13.10",
  hasStore: false,        // Alpine.store('session') === undefined
  bootRan: true,           // app.js IIFE ran; URL stripped of ?t=
  tokenStored: true,
  hasBootApp: true
  ```

## What Didn't Work

No dead ends to report — the script-order hypothesis was confirmed on the first reproduction. The `browser_evaluate` snapshot (`hasAlpine: true` + `hasStore: false` + `bootRan: true`) made it clear both scripts executed but in the wrong order relative to `alpine:init`, so no time was spent on CSS, the bearer-token middleware, the WebSocket handler, `embed.FS` wiring, or `?t=` query handling.

## Solution

Swap the two `<script>` tags in `internal/web/dist/index.html` so the ES module loads before the deferred Alpine classic script.

Before:

```html
<script src="/vendor/alpine.min.js" defer></script>
<script type="module" src="/js/app.js"></script>
```

After:

```html
<script type="module" src="/js/app.js"></script>
<script src="/vendor/alpine.min.js" defer></script>
```

Rebuild required because the SPA is embedded via `//go:embed all:dist`:

```bash
go build -o /tmp/deezer-remote ./cmd/deezer-remote
```

Post-fix verification on both `/?role=player` and `/?role=controller`: 0 console errors, 0 warnings, `Alpine.store('session')` populated with `{connected, current, queue, queue_pos, position_ms, paused, volume, init}`, player shell visible with the "Waiting for a track…" placeholder.

## Why This Works

Per the HTML spec, both deferred classic scripts and ES module scripts execute after the parser finishes, in source order, before `DOMContentLoaded`. Alpine 3 ends `alpine.min.js`'s evaluation by calling `queueMicrotask(start)`; the microtask queue flushes between sibling script evaluations, so `Alpine.start()` (which dispatches `alpine:init`) fires *before* the next script in document order runs. With Alpine listed first, `app.js`'s `document.addEventListener("alpine:init", …)` attaches after the event has already fired, so `Alpine.data("appRoot", …)` and `registerStores(Alpine)` never run. Putting the module first inverts the sequence: `app.js` evaluates → listener is attached → Alpine evaluates → microtask fires `alpine:init` → listener registers the component and stores → Alpine walks the DOM and `x-data="appRoot"`, `x-show`, `x-text` all resolve.

## Prevention

- **Source-order rule for Alpine 3 + ES modules.** When bootstrapping Alpine from an ES module (`<script type="module">`), the module's `<script>` tag must appear in source order *before* the Alpine `<script>` tag, or the registration must run from a classic inline `<script>` that precedes the Alpine tag. The `defer` attribute does not save you — microtasks fire between sibling scripts.
- **Alternative: take manual control of `Alpine.start()`.** Set `window.deferLoadingAlpine = (callback) => { /* call later */ }` from a classic inline `<script>` that precedes `alpine.min.js`. This is explicit but adds a moving part; prefer the source-order fix unless you genuinely need to gate startup on async work.
- **Smoke test on every `serve`.** A headless-browser check that opens the served page and asserts:
  1. `typeof Alpine.store('session') === 'object'` and contains the expected keys.
  2. The page text contains a string rendered by an `x-text` binding (e.g. the "Waiting for a track…" placeholder, or any visible label that only appears when Alpine resolved its bindings).
  3. `console` has zero errors during load.

  A one-shot Playwright pass against `deezer-remote serve` is enough to catch this regression class — silent Alpine-init failures only show up at runtime and are invisible to `go test`.
- **Treat embedded assets as binary-rebuild-required.** Any change under `internal/web/dist/` needs `go build` before retesting; an unchanged `./deezer-remote` will keep serving the old `index.html` and mask both the bug and the fix.

## Related Issues

- None. First frontend/SPA learning in `docs/solutions/`.
