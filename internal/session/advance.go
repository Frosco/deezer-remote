package session

// MaxConsecutiveSkips is the upper bound on consecutive failed loads before
// the session pauses and the transport emits a queue_exhausted error.
const MaxConsecutiveSkips = 5

// SkipDead advances the queue after a failed track load. Returns:
//   - next track if there is one and the cap has not been reached
//   - ok=true when a next track is available AND skipCount < MaxConsecutiveSkips
//   - exhausted=true when the cap is reached OR the queue is empty
//
// On exhausted=true the session is paused; callers should emit
// {kind: "queue_exhausted"} over the wire.
func (s *Session) SkipDead() (Track, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.skipCount++
	if s.skipCount >= MaxConsecutiveSkips {
		s.state.Paused = true
		return Track{}, false, true
	}
	if s.state.QueuePos+1 >= len(s.state.Queue) {
		s.state.Paused = true
		return Track{}, false, true
	}
	s.state.QueuePos++
	cur := s.state.Queue[s.state.QueuePos]
	s.state.Current = &cur
	s.state.PositionMs = 0
	s.state.Paused = false
	return cur, true, false
}

// MarkLoaded resets the consecutive-skip counter. Called by transport once a
// track has been successfully resolved and started playing.
func (s *Session) MarkLoaded() {
	s.mu.Lock()
	s.skipCount = 0
	s.mu.Unlock()
}
