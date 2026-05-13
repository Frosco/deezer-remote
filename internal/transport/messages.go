// Package transport owns the HTTP + WebSocket surface that controllers and
// the player tab connect to. It depends on internal/session and
// internal/media, never the other way round.
package transport

import (
	"encoding/json"

	"github.com/niref/deezer-remote/internal/session"
)

// Error.Kind values used on the wire. Controllers and the player branch on
// these strings — never on the human-readable Message.
const (
	ErrKindAuthExpired    = "auth_expired"
	ErrKindNotAvailable   = "not_available"
	ErrKindRateLimited    = "rate_limited"
	ErrKindPlayerGone     = "player_gone"
	ErrKindRegionLocked   = "region_locked"
	ErrKindQueueExhausted = "queue_exhausted"
	ErrKindRoleTaken      = "role_taken"
	ErrKindInternal       = "internal"
)

// Role values for the initial "hello" message.
const (
	RolePlayer     = "player"
	RoleController = "controller"
)

// Cmd kinds (controller → service).
const (
	CmdPlayTrack    = "play_track"
	CmdPlayAlbum    = "play_album"
	CmdPlayPlaylist = "play_playlist"
	CmdPlay         = "play"
	CmdPause        = "pause"
	CmdNext         = "next"
	CmdPrev         = "prev"
	CmdSeek         = "seek"
	CmdSetVolume    = "set_volume"
)

// Do kinds (service → player).
const (
	DoLoad      = "load"
	DoPlay      = "play"
	DoPause     = "pause"
	DoSeek      = "seek"
	DoSetVolume = "set_volume"
)

// Message envelope. Every WS message has {"type": "..."} as its first field.
type Message struct {
	Type string `json:"type"`
}

// Hello is the first message a tab sends after connecting.
type Hello struct {
	Type string `json:"type"` // "hello"
	Role string `json:"role"` // "player" | "controller"
}

// PlaybackUpdate is sent by the player tab every ~250 ms.
type PlaybackUpdate struct {
	Type       string `json:"type"` // "playback"
	PositionMs int64  `json:"position_ms"`
	Paused     bool   `json:"paused"`
	Ended      bool   `json:"ended"`
}

// Cmd is a controller-issued command.
type Cmd struct {
	Type    string          `json:"type"` // "cmd"
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Do is a service-issued instruction to the player tab.
type Do struct {
	Type    string          `json:"type"` // "do"
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// LoadPayload is the body of a Do{Kind: DoLoad}.
type LoadPayload struct {
	TrackID   string `json:"track_id"`
	StreamURL string `json:"stream_url"` // /stream/<id>?t=<token>
}

// SeekPayload is the body of Cmd{Kind: CmdSeek} and Do{Kind: DoSeek}.
type SeekPayload struct {
	PositionMs int64 `json:"position_ms"`
}

// VolumePayload is the body of Cmd{Kind: CmdSetVolume} and Do{Kind: DoSetVolume}.
type VolumePayload struct {
	Volume float64 `json:"volume"`
}

// PlayTrackPayload is the body of Cmd{Kind: CmdPlayTrack}.
type PlayTrackPayload struct {
	TrackID string `json:"track_id"`
}

// PlayAlbumPayload is the body of Cmd{Kind: CmdPlayAlbum}.
type PlayAlbumPayload struct {
	AlbumID  string `json:"album_id"`
	StartIdx int    `json:"start_idx,omitempty"`
}

// PlayPlaylistPayload is the body of Cmd{Kind: CmdPlayPlaylist}.
type PlayPlaylistPayload struct {
	PlaylistID string `json:"playlist_id"`
	StartIdx   int    `json:"start_idx,omitempty"`
}

// StateUpdate is the state snapshot fanned out to all tabs.
type StateUpdate struct {
	Type  string        `json:"type"` // "state"
	State session.State `json:"state"`
}

// ErrorMessage is the classified-kind error envelope.
type ErrorMessage struct {
	Type    string `json:"type"` // "error"
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// CoverURL builds the image-CDN URL for a track/album cover md5 hash.
// Phase 1 uses 250x250 for both phone and laptop.
func CoverURL(md5 string) string {
	if md5 == "" {
		return ""
	}
	return "https://e-cdns-images.dzcdn.net/images/cover/" + md5 + "/250x250-000000-80-0-0.jpg"
}
