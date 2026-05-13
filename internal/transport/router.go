package transport

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/session"
)

// HubSink is the subset of Hub the router talks to. Defined so router_test
// can use a fakeHub.
type HubSink interface {
	Broadcast(v any)
	SendToPlayer(v any) bool
	HasPlayer() bool
}

// CmdRouter applies controller cmds to the session, instructs the player,
// and broadcasts state changes.
type CmdRouter struct {
	sess  *session.Session
	gw    GatewayAPI
	res   Resolver
	hub   HubSink
	token string
}

// NewRouter wires a router. token is the bearer token; it's appended to
// /stream/<id> URLs so the player's <audio> tag can fetch them.
func NewRouter(sess *session.Session, gw GatewayAPI, res Resolver, hub HubSink, token string) *CmdRouter {
	return &CmdRouter{sess: sess, gw: gw, res: res, hub: hub, token: token}
}

// SetHub replaces the router's hub (used during server bootstrap).
func (r *CmdRouter) SetHub(h HubSink) { r.hub = h }

// State returns the current state snapshot wrapped in a StateUpdate.
func (r *CmdRouter) State() any {
	return StateUpdate{Type: "state", State: r.sess.State()}
}

// OnCmd applies a single controller command.
func (r *CmdRouter) OnCmd(c Cmd) {
	switch c.Kind {
	case CmdPlayTrack:
		var p PlayTrackPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		go r.playTrack(p.TrackID)
	case CmdPlayAlbum:
		var p PlayAlbumPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		go r.playAlbum(p.AlbumID, p.StartIdx)
	case CmdPlayPlaylist:
		var p PlayPlaylistPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		go r.playPlaylist(p.PlaylistID, p.StartIdx)
	case CmdPlay:
		r.sess.Play()
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoPlay})
		r.broadcastState()
	case CmdPause:
		r.sess.Pause()
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoPause})
		r.broadcastState()
	case CmdNext:
		go r.advance(false)
	case CmdPrev:
		if t, ok := r.sess.Prev(); ok {
			r.loadAndBroadcast(t.ID)
		}
	case CmdSeek:
		var p SeekPayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		r.sess.SeekTo(p.PositionMs)
		pb, _ := json.Marshal(p)
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoSeek, Payload: pb})
		r.broadcastState()
	case CmdSetVolume:
		var p VolumePayload
		if err := json.Unmarshal(c.Payload, &p); err != nil {
			return
		}
		r.sess.SetVolume(p.Volume)
		pb, _ := json.Marshal(p)
		r.hub.SendToPlayer(Do{Type: "do", Kind: DoSetVolume, Payload: pb})
		r.broadcastState()
	}
}

// OnPlayback is called for every {type:"playback"} message from the player.
func (r *CmdRouter) OnPlayback(p PlaybackUpdate) {
	r.sess.UpdatePlayback(p.PositionMs, p.Paused, p.Ended)
	if p.Ended {
		go r.advance(false)
	} else {
		r.broadcastState()
	}
}

// playTrack resolves a single track and starts it.
func (r *CmdRouter) playTrack(trackID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	td, err := r.gw.SongGetData(ctx, trackID)
	if err != nil {
		r.broadcastError(ErrKindNotAvailable, err.Error())
		return
	}
	r.sess.PlayTrack(toSessionTrack(td))
	r.loadAndBroadcast(trackID)
}

func (r *CmdRouter) playAlbum(albumID string, startIdx int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := r.gw.Album(ctx, albumID)
	if err != nil {
		r.broadcastError(ErrKindNotAvailable, err.Error())
		return
	}
	r.sess.PlayList(summariesToSessionTracks(a.Tracks), startIdx)
	if cur := r.sess.State().Current; cur != nil {
		r.loadAndBroadcast(cur.ID)
	}
}

func (r *CmdRouter) playPlaylist(playlistID string, startIdx int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := r.gw.Playlist(ctx, playlistID)
	if err != nil {
		r.broadcastError(ErrKindNotAvailable, err.Error())
		return
	}
	r.sess.PlayList(summariesToSessionTracks(p.Tracks), startIdx)
	if cur := r.sess.State().Current; cur != nil {
		r.loadAndBroadcast(cur.ID)
	}
}

// advance moves forward in the queue. If onDeadTrack, it counts as a skip.
func (r *CmdRouter) advance(onDeadTrack bool) {
	if onDeadTrack {
		_, ok, exhausted := r.sess.SkipDead()
		if exhausted {
			r.broadcastError(ErrKindQueueExhausted, "couldn't find a playable track in this queue")
			return
		}
		if !ok {
			return
		}
	} else {
		_, ok := r.sess.Next()
		if !ok {
			return
		}
	}
	cur := r.sess.State().Current
	if cur != nil {
		r.loadAndBroadcast(cur.ID)
	}
}

// loadAndBroadcast resolves a track, instructs the player to load it, and
// broadcasts the new state. On failure: skips and recurses with the cap.
func (r *CmdRouter) loadAndBroadcast(trackID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rt, err := r.res.Resolve(ctx, trackID)
	if err != nil {
		log.Printf("resolve %s: %v", trackID, err)
		r.broadcastError(ErrKindNotAvailable, "track unavailable: "+trackID)
		r.advance(true) // counts as skip
		return
	}
	streamURL := "/stream/" + trackID + "?t=" + r.token
	pb, _ := json.Marshal(LoadPayload{TrackID: trackID, StreamURL: streamURL})
	r.hub.SendToPlayer(Do{Type: "do", Kind: DoLoad, Payload: pb})
	r.sess.MarkLoaded()
	r.broadcastState()
	_ = rt
}

func (r *CmdRouter) broadcastState() {
	r.hub.Broadcast(StateUpdate{Type: "state", State: r.sess.State()})
}

func (r *CmdRouter) broadcastError(kind, msg string) {
	r.hub.Broadcast(ErrorMessage{Type: "error", Kind: kind, Message: msg})
}

func toSessionTrack(td *gateway.TrackData) session.Track {
	return session.Track{
		ID: td.SngID, Title: td.Title, Artist: td.Artist, Album: td.Album,
		DurationS: td.DurationS, CoverURL: CoverURL(td.CoverMD5),
	}
}

func summariesToSessionTracks(in []gateway.TrackSummary) []session.Track {
	out := make([]session.Track, 0, len(in))
	for _, t := range in {
		out = append(out, session.Track{
			ID: t.ID, Title: t.Title, Artist: t.Artist, Album: t.Album,
			DurationS: t.DurationS, CoverURL: CoverURL(t.CoverMD5),
		})
	}
	return out
}

