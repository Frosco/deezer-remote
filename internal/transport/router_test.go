package transport

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/niref/deezer-remote/internal/gateway"
	"github.com/niref/deezer-remote/internal/media"
	"github.com/niref/deezer-remote/internal/session"
)

// fakeHub captures Broadcast / SendToPlayer for inspection.
type fakeHub struct {
	mu        sync.Mutex
	bc        []any
	toPlayer  []any
	hasPlayer bool
}

func (h *fakeHub) Broadcast(v any) {
	h.mu.Lock()
	h.bc = append(h.bc, v)
	h.mu.Unlock()
}
func (h *fakeHub) SendToPlayer(v any) bool {
	h.mu.Lock()
	h.toPlayer = append(h.toPlayer, v)
	h.mu.Unlock()
	return h.hasPlayer
}
func (h *fakeHub) HasPlayer() bool { return h.hasPlayer }

func TestRouter_PlayTrack_LoadsAndBroadcasts(t *testing.T) {
	s := session.New()
	gw := fakeGW{
		track: func(ctx context.Context, id string) (*gateway.TrackData, error) {
			return &gateway.TrackData{SngID: id, Title: "T", Artist: "A", Album: "Z", CoverMD5: "p", DurationS: 200}, nil
		},
	}
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{TrackID: id, SngID: id, URL: "https://cdn/x", Format: media.FormatMP3_320, Size: 100, ExpiryAt: time.Now().Add(time.Hour)}, nil
	}}
	hub := &fakeHub{hasPlayer: true}
	r := NewRouter(s, gw, res, hub, "TOKEN")

	payload, _ := json.Marshal(PlayTrackPayload{TrackID: "42"})
	r.OnCmd(Cmd{Type: "cmd", Kind: CmdPlayTrack, Payload: payload})

	time.Sleep(50 * time.Millisecond)

	hub.mu.Lock()
	defer hub.mu.Unlock()
	if len(hub.toPlayer) == 0 {
		t.Fatal("expected at least one Do message to player")
	}
	loadFound := false
	for _, v := range hub.toPlayer {
		d, _ := v.(Do)
		if d.Kind == DoLoad {
			loadFound = true
		}
	}
	if !loadFound {
		t.Error("no do:load sent to player")
	}
	if s.State().Current == nil || s.State().Current.ID != "42" {
		t.Errorf("session.Current = %+v", s.State().Current)
	}
}

func TestRouter_OnPlaybackEnded_Advances(t *testing.T) {
	s := session.New()
	s.PlayList([]session.Track{{ID: "1"}, {ID: "2"}}, 0)
	gw := fakeGW{}
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{TrackID: id, SngID: id, URL: "u"}, nil
	}}
	hub := &fakeHub{hasPlayer: true}
	r := NewRouter(s, gw, res, hub, "TOKEN")
	r.OnPlayback(PlaybackUpdate{Type: "playback", Ended: true})
	time.Sleep(50 * time.Millisecond)
	if s.State().Current == nil || s.State().Current.ID != "2" {
		t.Errorf("session.Current after ended = %+v", s.State().Current)
	}
}

func TestRouter_OnPlaybackEnded_NotAvailable_Skips(t *testing.T) {
	s := session.New()
	s.PlayList([]session.Track{{ID: "1"}, {ID: "2"}, {ID: "3"}}, 0)
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		if id == "2" {
			return media.ResolvedTrack{}, errors.New("not available")
		}
		return media.ResolvedTrack{TrackID: id, SngID: id, URL: "u"}, nil
	}}
	hub := &fakeHub{hasPlayer: true}
	r := NewRouter(s, fakeGW{}, res, hub, "TOKEN")

	r.OnPlayback(PlaybackUpdate{Type: "playback", Ended: true})
	time.Sleep(100 * time.Millisecond)

	if s.State().Current == nil || s.State().Current.ID != "3" {
		t.Errorf("session.Current = %+v", s.State().Current)
	}
	hub.mu.Lock()
	defer hub.mu.Unlock()
	sawNotAvail := false
	for _, v := range hub.bc {
		e, ok := v.(ErrorMessage)
		if ok && e.Kind == ErrKindNotAvailable {
			sawNotAvail = true
		}
	}
	if !sawNotAvail {
		t.Error("expected a broadcasted not_available error")
	}
}

func TestRouter_State_ReturnsStateMessage(t *testing.T) {
	s := session.New()
	r := NewRouter(s, fakeGW{}, fakeResolver{}, &fakeHub{}, "TOKEN")
	v := r.State()
	su, ok := v.(StateUpdate)
	if !ok {
		t.Fatalf("State() = %T, want StateUpdate", v)
	}
	if su.Type != "state" {
		t.Errorf("Type = %q", su.Type)
	}
}
