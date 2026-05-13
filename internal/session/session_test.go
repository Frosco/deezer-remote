package session

import (
	"testing"
)

func tr(id, title string) Track {
	return Track{ID: id, Title: title, DurationS: 200}
}

func TestSession_PlayTrack_ReplacesQueue(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	st := s.State()
	if st.Current == nil || st.Current.ID != "1" {
		t.Errorf("Current = %+v", st.Current)
	}
	if len(st.Queue) != 1 || st.QueuePos != 0 {
		t.Errorf("Queue = %v, Pos = %d", st.Queue, st.QueuePos)
	}
	if st.Paused {
		t.Error("Paused should be false on PlayTrack")
	}
}

func TestSession_PlayList_StartsAtIndex(t *testing.T) {
	s := New()
	tracks := []Track{tr("1", "A"), tr("2", "B"), tr("3", "C")}
	s.PlayList(tracks, 1)
	st := s.State()
	if st.Current.ID != "2" || st.QueuePos != 1 || len(st.Queue) != 3 {
		t.Errorf("state = %+v", st)
	}
}

func TestSession_PauseResume(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	s.Pause()
	if !s.State().Paused {
		t.Error("Pause not reflected")
	}
	s.Play()
	if s.State().Paused {
		t.Error("Play did not unpause")
	}
}

func TestSession_Next_Prev(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A"), tr("2", "B"), tr("3", "C")}, 0)
	next, ok := s.Next()
	if !ok || next.ID != "2" {
		t.Errorf("Next = %+v ok=%v", next, ok)
	}
	prev, ok := s.Prev()
	if !ok || prev.ID != "1" {
		t.Errorf("Prev = %+v ok=%v", prev, ok)
	}
}

func TestSession_NextAtEnd_NotOK(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A")}, 0)
	if _, ok := s.Next(); ok {
		t.Error("Next at end should be !ok")
	}
}

func TestSession_SeekTo_ClampsAndStores(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	s.SeekTo(1234)
	if pos := s.State().PositionMs; pos != 1234 {
		t.Errorf("PositionMs = %d", pos)
	}
	s.SeekTo(-5)
	if pos := s.State().PositionMs; pos != 0 {
		t.Errorf("Negative seek not clamped: %d", pos)
	}
}

func TestSession_SetVolume_ClampedTo01(t *testing.T) {
	s := New()
	s.SetVolume(-1)
	if s.State().Volume != 0 {
		t.Errorf("Volume = %v", s.State().Volume)
	}
	s.SetVolume(2)
	if s.State().Volume != 1 {
		t.Errorf("Volume = %v", s.State().Volume)
	}
	s.SetVolume(0.5)
	if s.State().Volume != 0.5 {
		t.Errorf("Volume = %v", s.State().Volume)
	}
}

func TestSession_UpdatePlayback_StoresPosition(t *testing.T) {
	s := New()
	s.PlayTrack(tr("1", "A"))
	s.UpdatePlayback(5000, false, false)
	if pos := s.State().PositionMs; pos != 5000 {
		t.Errorf("PositionMs = %d", pos)
	}
	s.UpdatePlayback(0, true, false)
	if !s.State().Paused {
		t.Error("Paused not reflected from UpdatePlayback")
	}
}
