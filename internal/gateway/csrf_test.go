package gateway

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

// userDataResp builds a minimal deezer.getUserData success body.
func userDataResp(checkForm string, userID int) string {
	return `{"error":[],"results":{"USER":{"USER_ID":` +
		strconv.Itoa(userID) +
		`,"OPTIONS":{"license_token":"LT"}},"checkForm":"` +
		checkForm +
		`"}}`
}

func TestRefreshCSRF_StoresApiToken(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Bootstrap call must use api_token=null.
		if !strings.Contains(req.URL.RawQuery, "api_token=null") {
			t.Errorf("bootstrap should use api_token=null, got %s", req.URL.RawQuery)
		}
		return mkResp(200, userDataResp("TOK123", 42)), nil
	})

	c, _ := newClientWithTransport("ARL", rt)
	if err := c.refreshCSRF(context.Background()); err != nil {
		t.Fatalf("refreshCSRF: %v", err)
	}
	if c.apiToken != "TOK123" {
		t.Errorf("apiToken = %q, want %q", c.apiToken, "TOK123")
	}
}

func TestRefreshCSRF_UserID0IsAuthFailed(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		// USER_ID==0 is gw-light's signal that the arl is invalid.
		return mkResp(200, userDataResp("WHATEVER", 0)), nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	err := c.refreshCSRF(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("err = %v, want ErrAuthFailed", err)
	}
}

func TestCallWithCSRF_BootstrapsThenCalls(t *testing.T) {
	calls := int32(0)
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1:
			// Bootstrap getUserData.
			if !strings.Contains(req.URL.RawQuery, "method=deezer.getUserData") {
				t.Errorf("call 1 should be getUserData, got %s", req.URL.RawQuery)
			}
			return mkResp(200, userDataResp("TOK", 42)), nil
		case 2:
			// Actual method, with refreshed token.
			if !strings.Contains(req.URL.RawQuery, "api_token=TOK") {
				t.Errorf("call 2 should use api_token=TOK, got %s", req.URL.RawQuery)
			}
			if !strings.Contains(req.URL.RawQuery, "method=song.getData") {
				t.Errorf("call 2 should be song.getData, got %s", req.URL.RawQuery)
			}
			return mkResp(200, `{"error":[],"results":{"SNG_ID":"1"}}`), nil
		}
		t.Fatalf("unexpected call %d", n)
		return nil, nil
	})

	c, _ := newClientWithTransport("ARL", rt)
	raw, err := c.callWithCSRF(context.Background(), "song.getData", map[string]any{"SNG_ID": "1"})
	if err != nil {
		t.Fatalf("callWithCSRF: %v", err)
	}
	if !strings.Contains(string(raw), `"SNG_ID":"1"`) {
		t.Errorf("results %s missing SNG_ID", raw)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", got)
	}
}

func TestCallWithCSRF_RetriesOnceOnExpiredCSRF(t *testing.T) {
	calls := int32(0)
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1:
			// Bootstrap.
			return mkResp(200, userDataResp("OLDTOK", 42)), nil
		case 2:
			// First attempt: CSRF expired.
			if !strings.Contains(req.URL.RawQuery, "api_token=OLDTOK") {
				t.Errorf("call 2 should use OLDTOK, got %s", req.URL.RawQuery)
			}
			return mkResp(200, `{"error":{"CSRF_TOKEN_INVALID":"..."},"results":{}}`), nil
		case 3:
			// Re-bootstrap.
			return mkResp(200, userDataResp("NEWTOK", 42)), nil
		case 4:
			// Retry with new token.
			if !strings.Contains(req.URL.RawQuery, "api_token=NEWTOK") {
				t.Errorf("call 4 should use NEWTOK, got %s", req.URL.RawQuery)
			}
			return mkResp(200, `{"error":[],"results":{"ok":true}}`), nil
		}
		t.Fatalf("unexpected call %d", n)
		return nil, nil
	})

	c, _ := newClientWithTransport("ARL", rt)
	_, err := c.callWithCSRF(context.Background(), "song.getData", nil)
	if err != nil {
		t.Fatalf("callWithCSRF: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("expected 4 HTTP calls, got %d", got)
	}
}

func TestCallWithCSRF_DoesNotRetryTwice(t *testing.T) {
	calls := int32(0)
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1, 3:
			return mkResp(200, userDataResp("TOK"+strconv.Itoa(int(n)), 42)), nil
		default:
			// Always expired.
			return mkResp(200, `{"error":{"CSRF_TOKEN_INVALID":"..."},"results":{}}`), nil
		}
	})

	c, _ := newClientWithTransport("ARL", rt)
	_, err := c.callWithCSRF(context.Background(), "song.getData", nil)
	if !errors.Is(err, ErrCSRFExpired) {
		t.Errorf("err = %v, want ErrCSRFExpired after exhausted retry", err)
	}
	// Total: bootstrap(1) + call(2 expired) + refresh(3) + retry(4 expired) = 4.
	if got := atomic.LoadInt32(&calls); got != 4 {
		t.Errorf("expected 4 HTTP calls, got %d", got)
	}
}
