package media

import (
	"testing"
	"time"
)

func TestCache_PutGet(t *testing.T) {
	c := NewCache(30 * time.Minute)
	now := time.Unix(1_000_000, 0)
	c.clock = func() time.Time { return now }

	r := ResolvedTrack{
		TrackID:  "42",
		SngID:    "42",
		URL:      "https://cdn/x",
		Format:   FormatMP3_320,
		Size:     9059264,
		ExpiryAt: now.Add(2 * time.Hour),
	}
	c.Put(r)
	got, ok := c.Get("42")
	if !ok || got.URL != "https://cdn/x" {
		t.Errorf("Get = %+v, ok=%v", got, ok)
	}
}

func TestCache_RefetchWhenInsideSafetyMargin(t *testing.T) {
	c := NewCache(30 * time.Minute)
	now := time.Unix(1_000_000, 0)
	c.clock = func() time.Time { return now }
	c.Put(ResolvedTrack{TrackID: "42", URL: "u", ExpiryAt: now.Add(29 * time.Minute)})

	_, ok := c.Get("42")
	if ok {
		t.Errorf("entry expiring in 29 min must be considered stale (margin=30m)")
	}
	// And evicted.
	c.Put(ResolvedTrack{TrackID: "42", URL: "u2", ExpiryAt: now.Add(31 * time.Minute)})
	if got, ok := c.Get("42"); !ok || got.URL != "u2" {
		t.Errorf("Get after re-put = %+v, ok=%v", got, ok)
	}
}

func TestCache_Evict(t *testing.T) {
	c := NewCache(30 * time.Minute)
	c.Put(ResolvedTrack{TrackID: "42", URL: "u", ExpiryAt: time.Now().Add(10 * time.Hour)})
	c.Evict("42")
	if _, ok := c.Get("42"); ok {
		t.Error("Evict failed")
	}
}

func TestCache_ConcurrentSafe(t *testing.T) {
	c := NewCache(30 * time.Minute)
	done := make(chan struct{})
	for i := 0; i < 20; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 100; j++ {
				c.Put(ResolvedTrack{TrackID: "x", URL: "u", ExpiryAt: time.Now().Add(time.Hour)})
				_, _ = c.Get("x")
				c.Evict("x")
			}
		}()
	}
	for i := 0; i < 20; i++ {
		<-done
	}
}
