package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/niref/deezer-remote/internal/gateway"
)

// GatewayAPI is the subset of gateway.Client the HTTP handlers need.
// Defined as an interface so api_test.go can substitute a fake.
type GatewayAPI interface {
	Search(ctx context.Context, query string, limit int) (*gateway.SearchResult, error)
	SongGetData(ctx context.Context, trackID string) (*gateway.TrackData, error)
	Album(ctx context.Context, albumID string) (*gateway.Album, error)
	Playlist(ctx context.Context, playlistID string) (*gateway.Playlist, error)
}

// APIHandlers wires /api/* endpoints over a GatewayAPI.
type APIHandlers struct {
	gw GatewayAPI
}

// NewAPIHandlers builds a set of handlers bound to gw.
func NewAPIHandlers(gw GatewayAPI) *APIHandlers {
	return &APIHandlers{gw: gw}
}

// Register attaches handlers to mux. Caller is responsible for wrapping with
// RequireToken middleware before mounting on the public listener.
func (h *APIHandlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/search", h.search)
	mux.HandleFunc("GET /api/track/{id}", h.track)
	mux.HandleFunc("GET /api/album/{id}", h.album)
	mux.HandleFunc("GET /api/playlist/{id}", h.playlist)
}

func (h *APIHandlers) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		http.Error(w, "missing q", http.StatusBadRequest)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}
	res, err := h.gw.Search(r.Context(), q, limit)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, searchToAPI(res))
}

func (h *APIHandlers) track(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	td, err := h.gw.SongGetData(r.Context(), id)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, trackDataToAPI(td))
}

func (h *APIHandlers) album(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := h.gw.Album(r.Context(), id)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, albumToAPI(a))
}

func (h *APIHandlers) playlist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := h.gw.Playlist(r.Context(), id)
	if err != nil {
		writeGatewayError(w, err)
		return
	}
	writeJSON(w, playlistToAPI(p))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeGatewayError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gateway.ErrAuthFailed):
		http.Error(w, ErrKindAuthExpired, http.StatusUnauthorized)
	case errors.Is(err, gateway.ErrNotFound):
		http.Error(w, "not_found", http.StatusNotFound)
	case errors.Is(err, gateway.ErrRateLimited):
		http.Error(w, ErrKindRateLimited, http.StatusTooManyRequests)
	default:
		http.Error(w, ErrKindInternal+": "+err.Error(), http.StatusInternalServerError)
	}
}

// Output shapes (subset of gateway types with CoverURL filled in).

type apiTrack struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	DurationS int    `json:"duration_s"`
	CoverURL  string `json:"cover_url"`
}

type apiAlbumHeader struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	CoverURL   string `json:"cover_url"`
	TrackCount int    `json:"track_count"`
}

type apiPlaylistHeader struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Owner      string `json:"owner"`
	CoverURL   string `json:"cover_url"`
	TrackCount int    `json:"track_count"`
}

type apiSearchResult struct {
	Tracks    []apiTrack          `json:"tracks"`
	Albums    []apiAlbumHeader    `json:"albums"`
	Playlists []apiPlaylistHeader `json:"playlists"`
}

type apiAlbum struct {
	Header apiAlbumHeader `json:"header"`
	Tracks []apiTrack     `json:"tracks"`
}

type apiPlaylist struct {
	Header apiPlaylistHeader `json:"header"`
	Tracks []apiTrack        `json:"tracks"`
}

func summaryToAPI(t gateway.TrackSummary) apiTrack {
	return apiTrack{
		ID: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album,
		DurationS: t.DurationS, CoverURL: CoverURL(t.CoverMD5),
	}
}

func albumHeaderToAPI(h gateway.AlbumSummary) apiAlbumHeader {
	return apiAlbumHeader{
		ID: h.ID, Title: h.Title, Artist: h.Artist,
		CoverURL: CoverURL(h.CoverMD5), TrackCount: h.TrackCount,
	}
}

func playlistHeaderToAPI(h gateway.PlaylistSummary) apiPlaylistHeader {
	return apiPlaylistHeader{
		ID: h.ID, Title: h.Title, Owner: h.Owner,
		CoverURL: CoverURL(h.CoverMD5), TrackCount: h.TrackCount,
	}
}

func searchToAPI(r *gateway.SearchResult) apiSearchResult {
	out := apiSearchResult{
		Tracks:    make([]apiTrack, 0, len(r.Tracks)),
		Albums:    make([]apiAlbumHeader, 0, len(r.Albums)),
		Playlists: make([]apiPlaylistHeader, 0, len(r.Playlists)),
	}
	for _, t := range r.Tracks {
		out.Tracks = append(out.Tracks, summaryToAPI(t))
	}
	for _, a := range r.Albums {
		out.Albums = append(out.Albums, albumHeaderToAPI(a))
	}
	for _, p := range r.Playlists {
		out.Playlists = append(out.Playlists, playlistHeaderToAPI(p))
	}
	return out
}

func albumToAPI(a *gateway.Album) apiAlbum {
	out := apiAlbum{Header: albumHeaderToAPI(a.Header), Tracks: make([]apiTrack, 0, len(a.Tracks))}
	for _, t := range a.Tracks {
		out.Tracks = append(out.Tracks, summaryToAPI(t))
	}
	return out
}

func playlistToAPI(p *gateway.Playlist) apiPlaylist {
	out := apiPlaylist{Header: playlistHeaderToAPI(p.Header), Tracks: make([]apiTrack, 0, len(p.Tracks))}
	for _, t := range p.Tracks {
		out.Tracks = append(out.Tracks, summaryToAPI(t))
	}
	return out
}

func trackDataToAPI(td *gateway.TrackData) apiTrack {
	return apiTrack{
		ID: td.SngID, Title: td.Title, Artist: td.Artist, Album: td.Album,
		DurationS: td.DurationS, CoverURL: CoverURL(td.CoverMD5),
	}
}
