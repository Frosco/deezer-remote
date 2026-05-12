// Package media handles Deezer's track-streaming endpoints: stream URL
// acquisition (media.getUrl) and on-the-fly Blowfish decryption.
package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const mediaGetURLEndpoint = "https://media.deezer.com/v1/get_url"

// Format names the audio formats we may request.
type Format string

const (
	FormatMP3_128 Format = "MP3_128"
	FormatMP3_320 Format = "MP3_320"
)

// URLRequest is the input to GetURL.
type URLRequest struct {
	LicenseToken string
	TrackToken   string
	Formats      []Format // preference order; first is preferred
}

// URLResult is the chosen CDN URL plus metadata.
type URLResult struct {
	URL    string
	Format Format
	Expiry int64 // unix seconds when the URL expires
}

// Client speaks to media.deezer.com/v1/get_url.
type Client struct {
	http *http.Client
}

// NewClient builds a Client with the given transport.
func NewClient(rt http.RoundTripper) *Client {
	return &Client{http: &http.Client{Transport: rt}}
}

type urlReqWire struct {
	LicenseToken string      `json:"license_token"`
	TrackTokens  []string    `json:"track_tokens"`
	Media        []mediaSpec `json:"media"`
}

type mediaSpec struct {
	Type    string       `json:"type"`
	Formats []formatSpec `json:"formats"`
}

type formatSpec struct {
	Cipher string `json:"cipher"`
	Format string `json:"format"`
}

type urlRespWire struct {
	Data []struct {
		Media []struct {
			MediaType string `json:"media_type"`
			Cipher    struct {
				Type string `json:"type"`
			} `json:"cipher"`
			Format  string `json:"format"`
			Sources []struct {
				URL string `json:"url"`
			} `json:"sources"`
			Expiry int64 `json:"exp"`
		} `json:"media"`
		Errors []struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"data"`
}

// GetURL exchanges a track token + license token for a fresh CDN URL.
func (c *Client) GetURL(ctx context.Context, in URLRequest) (*URLResult, error) {
	formats := make([]formatSpec, 0, len(in.Formats))
	for _, f := range in.Formats {
		formats = append(formats, formatSpec{Cipher: "BF_CBC_STRIPE", Format: string(f)})
	}

	wire := urlReqWire{
		LicenseToken: in.LicenseToken,
		TrackTokens:  []string{in.TrackToken},
		Media: []mediaSpec{{
			Type:    "FULL",
			Formats: formats,
		}},
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mediaGetURLEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post get_url: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("get_url: http %d", resp.StatusCode)
	}

	var w urlRespWire
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil {
		return nil, fmt.Errorf("decode get_url response: %w", err)
	}

	if len(w.Data) == 0 {
		return nil, fmt.Errorf("get_url: empty data")
	}
	if len(w.Data[0].Errors) > 0 {
		e := w.Data[0].Errors[0]
		return nil, fmt.Errorf("get_url: track unavailable (code %d: %s)", e.Code, e.Message)
	}
	if len(w.Data[0].Media) == 0 {
		return nil, fmt.Errorf("get_url: no media returned (no format match for tier?)")
	}
	m := w.Data[0].Media[0]
	if len(m.Sources) == 0 {
		return nil, fmt.Errorf("get_url: media has no sources")
	}
	return &URLResult{
		URL:    m.Sources[0].URL,
		Format: Format(m.Format),
		Expiry: m.Expiry,
	}, nil
}
