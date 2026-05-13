// Package session owns the authoritative remote-control state: current
// track, queue, paused intent, volume target, and auto-advance bookkeeping.
package session

// Track is the wire-friendly track shape exchanged with controllers and
// the player tab. CoverURL is a fully-formed image URL (the transport
// layer fills it in from cover_md5 when adapting from gateway summaries).
type Track struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	DurationS int    `json:"duration_s"`
	CoverURL  string `json:"cover_url"`
}
