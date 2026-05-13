package gateway

import (
	"context"
	"encoding/json"
)

// Playlist is a playlist header plus its tracklist.
type Playlist struct {
	Header PlaylistSummary `json:"header"`
	Tracks []TrackSummary  `json:"tracks"`
}

type playlistHeaderWire struct {
	PlaylistID flexString `json:"PLAYLIST_ID"`
	Title      string     `json:"TITLE"`
	Owner      string     `json:"PARENT_USERNAME"`
	Pic        string     `json:"PLAYLIST_PICTURE"`
	NbSong     flexString `json:"NB_SONG"`
}

type playlistSongsWire struct {
	Data []struct {
		SngID    flexString `json:"SNG_ID"`
		Title    string     `json:"SNG_TITLE"`
		Artist   string     `json:"ART_NAME"`
		Album    string     `json:"ALB_TITLE"`
		AlbPic   string     `json:"ALB_PICTURE"`
		Duration flexString `json:"DURATION"`
	} `json:"data"`
}

// Playlist fetches the playlist header and tracklist.
func (c *Client) Playlist(ctx context.Context, playlistID string) (*Playlist, error) {
	rawHeader, err := c.callWithCSRF(ctx, "playlist.getData", map[string]any{"PLAYLIST_ID": playlistID})
	if err != nil {
		return nil, err
	}
	var h playlistHeaderWire
	if err := json.Unmarshal(rawHeader, &h); err != nil {
		return nil, err
	}
	if h.PlaylistID == "" {
		return nil, ErrNotFound
	}

	rawSongs, err := c.callWithCSRF(ctx, "playlist.getSongs", map[string]any{
		"PLAYLIST_ID": playlistID,
		"start":       0,
		"nb":          -1,
	})
	if err != nil {
		return nil, err
	}
	var sw playlistSongsWire
	if err := json.Unmarshal(rawSongs, &sw); err != nil {
		return nil, err
	}

	out := &Playlist{
		Header: PlaylistSummary{
			ID:         string(h.PlaylistID),
			Title:      h.Title,
			Owner:      h.Owner,
			CoverMD5:   h.Pic,
			TrackCount: atoiOrZero(string(h.NbSong)),
		},
		Tracks: make([]TrackSummary, 0, len(sw.Data)),
	}
	for _, t := range sw.Data {
		out.Tracks = append(out.Tracks, TrackSummary{
			ID:        string(t.SngID),
			Title:     t.Title,
			Artist:    t.Artist,
			Album:     t.Album,
			CoverMD5:  t.AlbPic,
			DurationS: atoiOrZero(string(t.Duration)),
		})
	}
	return out, nil
}
