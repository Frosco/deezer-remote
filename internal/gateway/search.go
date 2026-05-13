package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// SearchResult is the parsed payload for /api/search.
type SearchResult struct {
	Tracks    []TrackSummary    `json:"tracks"`
	Albums    []AlbumSummary    `json:"albums"`
	Playlists []PlaylistSummary `json:"playlists"`
}

// TrackSummary is a track in search results / album / playlist listings.
type TrackSummary struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	CoverMD5  string `json:"cover_md5"`
	DurationS int    `json:"duration_s"`
}

// AlbumSummary is an album in search results.
type AlbumSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Artist     string `json:"artist"`
	CoverMD5   string `json:"cover_md5"`
	TrackCount int    `json:"track_count"`
}

// PlaylistSummary is a playlist in search results.
type PlaylistSummary struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Owner      string `json:"owner"`
	CoverMD5   string `json:"cover_md5"`
	TrackCount int    `json:"track_count"`
}

type searchWire struct {
	Track struct {
		Data []struct {
			SngID    flexString `json:"SNG_ID"`
			Title    string     `json:"SNG_TITLE"`
			Artist   string     `json:"ART_NAME"`
			Album    string     `json:"ALB_TITLE"`
			AlbPic   string     `json:"ALB_PICTURE"`
			Duration flexString `json:"DURATION"`
		} `json:"data"`
	} `json:"TRACK"`
	Album struct {
		Data []struct {
			AlbID    flexString `json:"ALB_ID"`
			Title    string     `json:"ALB_TITLE"`
			Artist   string     `json:"ART_NAME"`
			AlbPic   string     `json:"ALB_PICTURE"`
			NumTrack flexString `json:"NUMBER_TRACK"`
		} `json:"data"`
	} `json:"ALBUM"`
	Playlist struct {
		Data []struct {
			PlaylistID flexString `json:"PLAYLIST_ID"`
			Title      string     `json:"TITLE"`
			Owner      string     `json:"PARENT_USERNAME"`
			Pic        string     `json:"PLAYLIST_PICTURE"`
			NbSong     flexString `json:"NB_SONG"`
		} `json:"data"`
	} `json:"PLAYLIST"`
}

// Search queries gw-light deezer.pageSearch and returns the parsed payload.
func (c *Client) Search(ctx context.Context, query string, limit int) (*SearchResult, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, errors.New("gateway: empty search query")
	}
	if limit <= 0 {
		limit = 20
	}
	raw, err := c.callWithCSRF(ctx, "deezer.pageSearch", map[string]any{
		"query":          q,
		"start":          0,
		"nb":             limit,
		"suggest":        true,
		"artist_suggest": false,
		"top_tracks":     true,
	})
	if err != nil {
		return nil, err
	}
	var w searchWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, err
	}
	out := &SearchResult{
		Tracks:    make([]TrackSummary, 0, len(w.Track.Data)),
		Albums:    make([]AlbumSummary, 0, len(w.Album.Data)),
		Playlists: make([]PlaylistSummary, 0, len(w.Playlist.Data)),
	}
	for _, t := range w.Track.Data {
		out.Tracks = append(out.Tracks, TrackSummary{
			ID:        string(t.SngID),
			Title:     t.Title,
			Artist:    t.Artist,
			Album:     t.Album,
			CoverMD5:  t.AlbPic,
			DurationS: atoiOrZero(string(t.Duration)),
		})
	}
	for _, a := range w.Album.Data {
		out.Albums = append(out.Albums, AlbumSummary{
			ID:         string(a.AlbID),
			Title:      a.Title,
			Artist:     a.Artist,
			CoverMD5:   a.AlbPic,
			TrackCount: atoiOrZero(string(a.NumTrack)),
		})
	}
	for _, p := range w.Playlist.Data {
		out.Playlists = append(out.Playlists, PlaylistSummary{
			ID:         string(p.PlaylistID),
			Title:      p.Title,
			Owner:      p.Owner,
			CoverMD5:   p.Pic,
			TrackCount: atoiOrZero(string(p.NbSong)),
		})
	}
	return out, nil
}
