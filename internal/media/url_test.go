package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGetURL_SendsRightBodyAndParsesResponse(t *testing.T) {
	var seenBody map[string]any
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://media.deezer.com/v1/get_url" {
			t.Errorf("URL = %q", req.URL)
		}
		if req.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", req.Method)
		}
		body, _ := io.ReadAll(req.Body)
		if err := json.Unmarshal(body, &seenBody); err != nil {
			t.Fatalf("body not JSON: %s", body)
		}
		respBody := `{"data":[{"media":[{"media_type":"FULL","cipher":{"type":"BF_CBC_STRIPE"},"format":"MP3_320","sources":[{"url":"https://cdn.example/track.bin","provider":"x"}],"nbf":1000,"exp":2000}]}]}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(respBody)),
			Header:     make(http.Header),
		}, nil
	})

	c := NewClient(rt)
	res, err := c.GetURL(context.Background(), URLRequest{
		LicenseToken: "LIC",
		TrackToken:   "TT-XYZ",
		Formats:      []Format{FormatMP3_320, FormatMP3_128},
	})
	if err != nil {
		t.Fatalf("GetURL: %v", err)
	}
	if res.URL != "https://cdn.example/track.bin" {
		t.Errorf("URL = %q", res.URL)
	}
	if res.Format != FormatMP3_320 {
		t.Errorf("Format = %q, want MP3_320", res.Format)
	}
	if res.Expiry != 2000 {
		t.Errorf("Expiry = %d, want 2000", res.Expiry)
	}

	// Body shape sanity.
	if seenBody["license_token"] != "LIC" {
		t.Errorf("body.license_token = %v", seenBody["license_token"])
	}
	tks, ok := seenBody["track_tokens"].([]any)
	if !ok || len(tks) != 1 || tks[0] != "TT-XYZ" {
		t.Errorf("body.track_tokens = %v", seenBody["track_tokens"])
	}
}

func TestGetURL_EmptyDataIsNotAvailable(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"errors":[{"code":2002,"message":"unavailable"}]}]}`)),
			Header:     make(http.Header),
		}, nil
	})
	c := NewClient(rt)
	_, err := c.GetURL(context.Background(), URLRequest{
		LicenseToken: "LIC",
		TrackToken:   "TT",
		Formats:      []Format{FormatMP3_128},
	})
	if err == nil {
		t.Fatal("expected error for empty media array")
	}
}
