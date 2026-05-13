package session

import "testing"

func TestSkipDead_Advances(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A"), tr("2", "B"), tr("3", "C")}, 0)
	next, ok, exhausted := s.SkipDead()
	if !ok || exhausted || next.ID != "2" {
		t.Errorf("SkipDead 1: next=%+v ok=%v exhausted=%v", next, ok, exhausted)
	}
}

func TestSkipDead_ExhaustsAtFifthConsecutive(t *testing.T) {
	s := New()
	tracks := []Track{tr("1", "A"), tr("2", "B"), tr("3", "C"), tr("4", "D"), tr("5", "E"), tr("6", "F"), tr("7", "G")}
	s.PlayList(tracks, 0)
	// Skip 5 times in a row.
	for i := 0; i < 4; i++ {
		_, ok, exhausted := s.SkipDead()
		if !ok || exhausted {
			t.Fatalf("skip %d: ok=%v exhausted=%v", i, ok, exhausted)
		}
	}
	_, ok, exhausted := s.SkipDead()
	if ok || !exhausted {
		t.Errorf("5th SkipDead: ok=%v exhausted=%v (want ok=false exhausted=true)", ok, exhausted)
	}
	// Exhausted pauses the session.
	if !s.State().Paused {
		t.Error("exhausted state should pause")
	}
}

func TestSkipDead_MarkLoadedResetsCounter(t *testing.T) {
	s := New()
	tracks := []Track{tr("1", "A"), tr("2", "B"), tr("3", "C"), tr("4", "D"), tr("5", "E"), tr("6", "F"), tr("7", "G")}
	s.PlayList(tracks, 0)
	for i := 0; i < 4; i++ {
		_, _, _ = s.SkipDead()
	}
	s.MarkLoaded()
	// After a successful load, the cap resets — we should be able to skip again.
	if _, ok, exhausted := s.SkipDead(); !ok || exhausted {
		t.Errorf("after MarkLoaded: ok=%v exhausted=%v", ok, exhausted)
	}
}

func TestSkipDead_NoNextTrack(t *testing.T) {
	s := New()
	s.PlayList([]Track{tr("1", "A")}, 0)
	_, ok, exhausted := s.SkipDead()
	if ok || !exhausted {
		t.Errorf("end-of-queue SkipDead: ok=%v exhausted=%v", ok, exhausted)
	}
}
