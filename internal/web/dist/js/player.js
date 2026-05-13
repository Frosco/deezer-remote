// Player tab: wraps the <audio> element, executes do-instructions from the
// service, and pushes playback updates every ~250 ms.
import { ws } from "./ws.js";

const PUSH_INTERVAL_MS = 250;

export function attachPlayer(audio) {
  let lastPushed = 0;

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

  ws.on("do", (m) => {
    switch (m.kind) {
      case "load":
        audio.src = m.payload.stream_url;
        audio.play().catch(err => console.warn("audio.play:", err));
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
