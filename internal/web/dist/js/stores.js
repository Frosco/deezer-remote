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
