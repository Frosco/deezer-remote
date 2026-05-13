package transport

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	heartbeatInterval = 10 * time.Second
	heartbeatTimeout  = 30 * time.Second
	writeTimeout      = 5 * time.Second
	readBufBytes      = 64 << 10
)

// Router is what the hub talks to when a tab sends a cmd or playback update.
// Implemented by the command router (Task 18). Decoupled so hub tests don't
// need a real session.
type Router interface {
	OnCmd(c Cmd)
	OnPlayback(p PlaybackUpdate)
	State() any // returns a value JSON-encoded as a "state" snapshot
}

// Hub owns live WebSocket connections. Implements http.Handler.
type Hub struct {
	r      Router
	mu     sync.Mutex
	player *conn // at most one
	ctrls  map[*conn]struct{}
	closed bool
}

// NewHub builds an empty hub bound to router r.
func NewHub(r Router) *Hub {
	return &Hub{r: r, ctrls: map[*conn]struct{}{}}
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // same-origin LAN; no Origin to enforce
	})
	if err != nil {
		return
	}
	c.SetReadLimit(readBufBytes)
	co := &conn{ws: c, hub: h, role: "", out: make(chan []byte, 32)}
	go co.writer()
	co.run(r.Context())
}

// Broadcast queues v on every controller (and the player). Drops on full
// buffer rather than blocking.
func (h *Hub) Broadcast(v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	h.mu.Lock()
	targets := make([]*conn, 0, len(h.ctrls)+1)
	if h.player != nil {
		targets = append(targets, h.player)
	}
	for c := range h.ctrls {
		targets = append(targets, c)
	}
	h.mu.Unlock()
	for _, c := range targets {
		select {
		case c.out <- b:
		default:
			// Drop; slow consumer.
		}
	}
}

// SendToPlayer queues v on the player conn, if any.
func (h *Hub) SendToPlayer(v any) bool {
	h.mu.Lock()
	p := h.player
	h.mu.Unlock()
	if p == nil {
		return false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	select {
	case p.out <- b:
		return true
	default:
		return false
	}
}

// HasPlayer reports whether a player tab is currently connected.
func (h *Hub) HasPlayer() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.player != nil
}

func (h *Hub) registerPlayer(c *conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.player != nil {
		return false
	}
	h.player = c
	return true
}

func (h *Hub) registerController(c *conn) {
	h.mu.Lock()
	h.ctrls[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) drop(c *conn) {
	h.mu.Lock()
	if h.player == c {
		h.player = nil
	}
	delete(h.ctrls, c)
	h.mu.Unlock()
}

// conn is one WS connection.
type conn struct {
	ws   *websocket.Conn
	hub  *Hub
	role string
	out  chan []byte
}

func (c *conn) writer() {
	for b := range c.out {
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		err := c.ws.Write(ctx, websocket.MessageText, b)
		cancel()
		if err != nil {
			return
		}
	}
}

func (c *conn) run(ctx context.Context) {
	defer func() {
		c.hub.drop(c)
		close(c.out)
		_ = c.ws.Close(websocket.StatusNormalClosure, "bye")
	}()

	// Heartbeat: ping every heartbeatInterval; close on timeout.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go c.heartbeat(ctx)

	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return
		}
		var env Message
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		switch env.Type {
		case "hello":
			var h Hello
			if err := json.Unmarshal(data, &h); err != nil {
				continue
			}
			c.role = h.Role
			c.onHello()
		case "cmd":
			var cm Cmd
			if err := json.Unmarshal(data, &cm); err != nil {
				continue
			}
			c.hub.r.OnCmd(cm)
		case "playback":
			var p PlaybackUpdate
			if err := json.Unmarshal(data, &p); err != nil {
				continue
			}
			c.hub.r.OnPlayback(p)
		}
	}
}

func (c *conn) onHello() {
	switch c.role {
	case RolePlayer:
		if !c.hub.registerPlayer(c) {
			b, _ := json.Marshal(ErrorMessage{Type: "error", Kind: ErrKindRoleTaken, Message: "another player is connected"})
			c.out <- b
			// Close shortly after so the dialer sees the error.
			go func() {
				time.Sleep(50 * time.Millisecond)
				_ = c.ws.Close(websocket.StatusPolicyViolation, "role_taken")
			}()
			return
		}
	case RoleController:
		c.hub.registerController(c)
	default:
		_ = c.ws.Close(websocket.StatusUnsupportedData, "bad role")
		return
	}
	b, _ := json.Marshal(c.hub.r.State())
	c.out <- b
}

func (c *conn) heartbeat(ctx context.Context) {
	t := time.NewTicker(heartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, heartbeatTimeout)
			err := c.ws.Ping(pctx)
			cancel()
			if err != nil {
				_ = c.ws.Close(websocket.StatusGoingAway, "heartbeat")
				return
			}
		}
	}
}
