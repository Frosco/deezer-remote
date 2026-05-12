package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// roundTripFunc adapts a closure into an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mkResp(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestCall_SendsCorrectRequest(t *testing.T) {
	var seenURL string
	var seenBody string
	var seenCookie string

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seenURL = req.URL.String()
		b, _ := io.ReadAll(req.Body)
		seenBody = string(b)
		seenCookie = req.Header.Get("Cookie")
		return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42}}}`), nil
	})

	c, err := newClientWithTransport("ARLVALUE", rt)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := c.Call(context.Background(), "deezer.getUserData", nil)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}

	// URL must include api_token and method as query params.
	if !strings.Contains(seenURL, "api_version=1.0") {
		t.Errorf("url missing api_version: %s", seenURL)
	}
	if !strings.Contains(seenURL, "method=deezer.getUserData") {
		t.Errorf("url missing method: %s", seenURL)
	}
	if !strings.Contains(seenURL, "api_token=null") {
		t.Errorf("url missing api_token=null for bootstrap call: %s", seenURL)
	}
	if seenBody != "{}" && seenBody != "null" {
		// nil params should serialise to either {} or null; tolerate either.
		t.Errorf("body for nil params = %q", seenBody)
	}
	if !strings.Contains(seenCookie, "arl=ARLVALUE") {
		t.Errorf("cookie missing arl: %q", seenCookie)
	}

	var parsed struct {
		USER struct {
			UserID int `json:"USER_ID"`
		} `json:"USER"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	if parsed.USER.UserID != 42 {
		t.Errorf("USER_ID = %d, want 42", parsed.USER.UserID)
	}
}

func TestCall_ParamsAreEncodedAsJSONBody(t *testing.T) {
	var seenBody string
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(req.Body)
		seenBody = string(b)
		return mkResp(200, `{"error":[],"results":{}}`), nil
	}) //

	c, _ := newClientWithTransport("ARL", rt)

	_, err := c.Call(context.Background(), "song.getData", map[string]any{"SNG_ID": "3135556"})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !strings.Contains(seenBody, `"SNG_ID":"3135556"`) {
		t.Errorf("body %q missing SNG_ID", seenBody)
	}
}

func TestCall_ClassifiesGatewayErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		responseJS string
		want       error
	}{
		{
			"csrf invalid in error array",
			200,
			`{"error":{"CSRF_TOKEN_INVALID":"..."},"results":{}}`,
			ErrCSRFExpired,
		},
		{
			"auth failed",
			200,
			`{"error":{"USER_AUTH_REQUIRED":"..."},"results":{}}`,
			ErrAuthFailed,
		},
		{
			"quota error",
			200,
			`{"error":{"QUOTA_ERROR":"..."},"results":{}}`,
			ErrRateLimited,
		},
		{
			"http 500",
			500,
			`internal server error`,
			ErrServerError,
		},
		{
			"http 429",
			429,
			`rate limited`,
			ErrRateLimited,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return mkResp(tc.status, tc.responseJS), nil
			})
			c, _ := newClientWithTransport("ARL", rt)
			_, err := c.Call(context.Background(), "song.getData", nil)
			if !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCall_CookieJarPersistsSID(t *testing.T) {
	calls := 0
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		resp := mkResp(200, `{"error":[],"results":{}}`)
		if calls == 1 {
			resp.Header.Set("Set-Cookie", "sid=ABC123; Path=/; Domain=deezer.com")
		}
		return resp, nil
	})

	c, _ := newClientWithTransport("ARL", rt)

	if _, err := c.Call(context.Background(), "deezer.getUserData", nil); err != nil {
		t.Fatal(err)
	}

	// Second call must include the sid cookie from the first response.
	var secondCookie string
	rt2 := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		secondCookie = req.Header.Get("Cookie")
		return mkResp(200, `{"error":[],"results":{}}`), nil
	})
	c.http.Transport = rt2
	if _, err := c.Call(context.Background(), "deezer.getUserData", nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(secondCookie, "sid=ABC123") {
		t.Errorf("second-call cookie %q does not contain sid=ABC123", secondCookie)
	}
}
