package transport

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func dial(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	c, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sendJSON(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := c.Write(context.Background(), websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

func readJSON(t *testing.T, c *websocket.Conn, out any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("unmarshal %q: %v", b, err)
	}
}

func TestHub_HelloPlayer_GetsStateSnapshot(t *testing.T) {
	hub := NewHub(nopRouter{})
	srv := httptest.NewServer(hub)
	defer srv.Close()
	c := dial(t, srv)
	defer c.Close(websocket.StatusNormalClosure, "")

	sendJSON(t, c, Hello{Type: "hello", Role: RolePlayer})
	var got map[string]any
	readJSON(t, c, &got)
	if got["type"] != "state" {
		t.Errorf("first msg = %v", got)
	}
}

func TestHub_SecondPlayer_GetsRoleTakenAndDisconnected(t *testing.T) {
	hub := NewHub(nopRouter{})
	srv := httptest.NewServer(hub)
	defer srv.Close()

	first := dial(t, srv)
	defer first.Close(websocket.StatusNormalClosure, "")
	sendJSON(t, first, Hello{Type: "hello", Role: RolePlayer})
	var ignored map[string]any
	readJSON(t, first, &ignored) // consume the state

	second := dial(t, srv)
	sendJSON(t, second, Hello{Type: "hello", Role: RolePlayer})
	var got map[string]any
	readJSON(t, second, &got)
	if got["type"] != "error" || got["kind"] != ErrKindRoleTaken {
		t.Errorf("second player got %v", got)
	}
}

func TestHub_BroadcastReachesAllControllers(t *testing.T) {
	hub := NewHub(nopRouter{})
	srv := httptest.NewServer(hub)
	defer srv.Close()

	a := dial(t, srv)
	defer a.Close(websocket.StatusNormalClosure, "")
	b := dial(t, srv)
	defer b.Close(websocket.StatusNormalClosure, "")

	sendJSON(t, a, Hello{Type: "hello", Role: RoleController})
	readJSON(t, a, &map[string]any{}) // consume welcome state
	sendJSON(t, b, Hello{Type: "hello", Role: RoleController})
	readJSON(t, b, &map[string]any{})

	hub.Broadcast(ErrorMessage{Type: "error", Kind: "internal", Message: "test"})
	var ga, gb map[string]any
	readJSON(t, a, &ga)
	readJSON(t, b, &gb)
	if ga["kind"] != "internal" || gb["kind"] != "internal" {
		t.Errorf("ga=%v gb=%v", ga, gb)
	}
}

// nopRouter is a Router that ignores everything.
type nopRouter struct{}

func (nopRouter) OnCmd(Cmd)                 {}
func (nopRouter) OnPlayback(PlaybackUpdate) {}
func (nopRouter) State() any                { return map[string]any{"type": "state"} }
