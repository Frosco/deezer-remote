// Player tab: wraps the <audio> element, executes do-instructions from the
// service, and pushes playback updates every ~250 ms.
import { ws } from "./ws.js";

const PUSH_INTERVAL_MS = 250;

export function attachPlayer(audio) {
  let lastPushed = 0;
  let pendingLoad = null;
  let unlocked = false;

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

  function startLoad(payload) {
    audio.src = payload.stream_url;
    audio.play().catch(err => console.warn("audio.play:", err));
  }

  // Browsers block audio.play() until the user has interacted with this tab.
  // The unlock button in the player shell fires "player-unlock"; the play()
  // call inside this handler runs in the click's stack frame so it's allowed.
  document.addEventListener("player-unlock", () => {
    if (unlocked) return;
    unlocked = true;
    if (pendingLoad) {
      const p = pendingLoad;
      pendingLoad = null;
      startLoad(p);
    }
  });

  ws.on("do", (m) => {
    switch (m.kind) {
      case "load":
        if (unlocked) {
          startLoad(m.payload);
        } else {
          pendingLoad = m.payload;
        }
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
