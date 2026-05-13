package session

import "sync"

// State is the snapshot every controller and the player render from.
type State struct {
	Current    *Track  `json:"current,omitempty"`
	Queue      []Track `json:"queue"`
	QueuePos   int     `json:"queue_pos"`
	PositionMs int64   `json:"position_ms"`
	Paused     bool    `json:"paused"`
	Volume     float64 `json:"volume"` // 0..1
}

// Session is the goroutine-safe owner of State.
type Session struct {
	mu    sync.RWMutex
	state State
	// skipCount tracks consecutive failed loads for the auto-advance cap.
	// See advance.go.
	skipCount int
}

// New returns a Session at volume 1.0, paused, with an empty queue.
func New() *Session {
	return &Session{state: State{Volume: 1.0, Paused: true}}
}

// State returns a deep-copied snapshot safe to marshal off the lock.
func (s *Session) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.state
	if len(s.state.Queue) > 0 {
		out.Queue = append([]Track(nil), s.state.Queue...)
	}
	if s.state.Current != nil {
		t := *s.state.Current
		out.Current = &t
	}
	return out
}

// PlayTrack replaces the queue with a single track and starts unpaused.
func (s *Session) PlayTrack(t Track) {
	s.PlayList([]Track{t}, 0)
}

// PlayList replaces the queue with tracks and starts at startIdx.
func (s *Session) PlayList(tracks []Track, startIdx int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(tracks) {
		startIdx = 0
	}
	s.state.Queue = append([]Track(nil), tracks...)
	s.state.QueuePos = startIdx
	if len(tracks) > 0 {
		cur := tracks[startIdx]
		s.state.Current = &cur
	} else {
		s.state.Current = nil
	}
	s.state.PositionMs = 0
	s.state.Paused = false
	s.skipCount = 0
}

// Pause sets the paused intent.
func (s *Session) Pause() { s.setPaused(true) }

// Play clears the paused intent.
func (s *Session) Play() { s.setPaused(false) }

func (s *Session) setPaused(v bool) {
	s.mu.Lock()
	s.state.Paused = v
	s.mu.Unlock()
}

// Next advances queue position by one and returns the new current track.
// Returns ok=false when already at the end.
func (s *Session) Next() (Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.QueuePos+1 >= len(s.state.Queue) {
		return Track{}, false
	}
	s.state.QueuePos++
	cur := s.state.Queue[s.state.QueuePos]
	s.state.Current = &cur
	s.state.PositionMs = 0
	s.state.Paused = false
	s.skipCount = 0
	return cur, true
}

// Prev steps queue position back by one.
func (s *Session) Prev() (Track, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.QueuePos == 0 {
		return Track{}, false
	}
	s.state.QueuePos--
	cur := s.state.Queue[s.state.QueuePos]
	s.state.Current = &cur
	s.state.PositionMs = 0
	s.state.Paused = false
	s.skipCount = 0
	return cur, true
}

// SeekTo clamps position to [0, +inf) and stores it.
func (s *Session) SeekTo(positionMs int64) {
	s.mu.Lock()
	if positionMs < 0 {
		positionMs = 0
	}
	s.state.PositionMs = positionMs
	s.mu.Unlock()
}

// SetVolume clamps v to [0, 1].
func (s *Session) SetVolume(v float64) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	s.mu.Lock()
	s.state.Volume = v
	s.mu.Unlock()
}

// UpdatePlayback ingests a periodic update from the player tab.
// ended=true triggers no state change here; the caller (transport) is
// responsible for invoking Next or SkipDead.
func (s *Session) UpdatePlayback(positionMs int64, paused bool, ended bool) {
	s.mu.Lock()
	if positionMs >= 0 {
		s.state.PositionMs = positionMs
	}
	s.state.Paused = paused
	s.mu.Unlock()
	_ = ended
}
