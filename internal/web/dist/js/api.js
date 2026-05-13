// Thin /api/* fetch helpers. The bearer token is read from localStorage
// (stored by app.js on first load).
const TOKEN_KEY = "deezer-remote/token";

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || "";
}

export function setToken(t) {
  localStorage.setItem(TOKEN_KEY, t);
}

async function get(path) {
  const r = await fetch(path, {
    headers: { Authorization: `Bearer ${getToken()}` },
  });
  if (!r.ok) throw new Error(`${path}: HTTP ${r.status}`);
  return r.json();
}

export const api = {
  search: (q) => get(`/api/search?q=${encodeURIComponent(q)}`),
  track:  (id) => get(`/api/track/${encodeURIComponent(id)}`),
  album:  (id) => get(`/api/album/${encodeURIComponent(id)}`),
  playlist: (id) => get(`/api/playlist/${encodeURIComponent(id)}`),
};
