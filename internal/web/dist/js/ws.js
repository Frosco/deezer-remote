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
