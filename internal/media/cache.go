package media

import (
	"sync"
	"time"
)

// ResolvedTrack is everything /stream/<id> needs to serve the track without
// re-running song.getData / media.getUrl.
type ResolvedTrack struct {
	TrackID  string
	SngID    string
	URL      string
	Format   Format
	Size     int64
	ExpiryAt time.Time
}

// Cache is an in-memory, goroutine-safe map of track_id → ResolvedTrack.
// Entries are considered stale when remaining TTL falls below SafetyMargin.
type Cache struct {
	mu           sync.Mutex
	entries      map[string]ResolvedTrack
	SafetyMargin time.Duration
	clock        func() time.Time
}

// NewCache builds an empty cache. safetyMargin is the minimum remaining TTL
// an entry must have to be considered fresh.
func NewCache(safetyMargin time.Duration) *Cache {
	return &Cache{
		entries:      make(map[string]ResolvedTrack),
		SafetyMargin: safetyMargin,
		clock:        time.Now,
	}
}

// Put stores or replaces the entry for r.TrackID.
func (c *Cache) Put(r ResolvedTrack) {
	c.mu.Lock()
	c.entries[r.TrackID] = r
	c.mu.Unlock()
}

// Get returns the entry for trackID if it exists and is still fresh.
// Stale entries are deleted as a side effect.
func (c *Cache) Get(trackID string) (ResolvedTrack, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.entries[trackID]
	if !ok {
		return ResolvedTrack{}, false
	}
	if r.ExpiryAt.Sub(c.clock()) < c.SafetyMargin {
		delete(c.entries, trackID)
		return ResolvedTrack{}, false
	}
	return r, true
}

// Evict removes the entry for trackID. Safe on a missing key.
func (c *Cache) Evict(trackID string) {
	c.mu.Lock()
	delete(c.entries, trackID)
	c.mu.Unlock()
}
