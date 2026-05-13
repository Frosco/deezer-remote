package gateway

import (
	"context"
	"encoding/json"
)

// Album is an album header plus its full tracklist.
type Album struct {
	Header AlbumSummary   `json:"header"`
	Tracks []TrackSummary `json:"tracks"`
}

type albumHeaderWire struct {
	AlbID    flexString `json:"ALB_ID"`
	Title    string     `json:"ALB_TITLE"`
	Artist   string     `json:"ART_NAME"`
	AlbPic   string     `json:"ALB_PICTURE"`
	NumTrack flexString `json:"NUMBER_TRACK"`
}

type albumTracksWire struct {
	Data []struct {
		SngID    flexString `json:"SNG_ID"`
		Title    string     `json:"SNG_TITLE"`
		Artist   string     `json:"ART_NAME"`
		Album    string     `json:"ALB_TITLE"`
		AlbPic   string     `json:"ALB_PICTURE"`
		Duration flexString `json:"DURATION"`
	} `json:"data"`
}

// Album fetches the album header and full tracklist in order.
func (c *Client) Album(ctx context.Context, albumID string) (*Album, error) {
	rawHeader, err := c.callWithCSRF(ctx, "album.getData", map[string]any{"ALB_ID": albumID})
	if err != nil {
		return nil, err
	}
	var h albumHeaderWire
	if err := json.Unmarshal(rawHeader, &h); err != nil {
		return nil, err
	}
	if h.AlbID == "" {
		return nil, ErrNotFound
	}

	rawTracks, err := c.callWithCSRF(ctx, "song.getListByAlbum", map[string]any{
		"ALB_ID": albumID,
		"start":  0,
		"nb":     -1,
	})
	if err != nil {
		return nil, err
	}
	var tw albumTracksWire
	if err := json.Unmarshal(rawTracks, &tw); err != nil {
		return nil, err
	}

	out := &Album{
		Header: AlbumSummary{
			ID:         string(h.AlbID),
			Title:      h.Title,
			Artist:     h.Artist,
			CoverMD5:   h.AlbPic,
			TrackCount: atoiOrZero(string(h.NumTrack)),
		},
		Tracks: make([]TrackSummary, 0, len(tw.Data)),
	}
	for _, t := range tw.Data {
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
