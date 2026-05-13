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
