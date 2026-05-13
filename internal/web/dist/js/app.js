// Bootstrap: strip token from URL on first load, register Alpine components
// and stores, connect the WS, and wire the audio element if we're in player
// role.
import { setToken, getToken } from "./api.js";
import { ws } from "./ws.js";
import { registerStores } from "./stores.js";
import { appRoot } from "./controller.js";
import { attachPlayer } from "./player.js";

(function bootToken() {
  const url = new URL(location.href);
  const t = url.searchParams.get("t");
  if (t) {
    setToken(t);
    url.searchParams.delete("t");
    history.replaceState({}, "", url.toString());
  }
})();

document.addEventListener("alpine:init", () => {
  window.Alpine.data("appRoot", appRoot);
  registerStores(window.Alpine);
});

window.__bootApp = function (store) {
  if (!getToken()) {
    console.error("no bearer token; re-open the URL with ?t= or run `deezer-remote pair`");
    return;
  }
  const role = document.body.classList.contains("player") ? "player" : "controller";
  // Note: body.classList isn't set yet at this point — use the heuristic.
  const detected = window.matchMedia("(min-width: 700px) and (pointer: fine)").matches
    ? "player" : "controller";
  const finalRole = new URLSearchParams(location.search).get("role") || detected;
  ws.connect(finalRole);
  if (finalRole === "player") {
    const audio = document.getElementById("audio");
    attachPlayer(audio);
  }
};
