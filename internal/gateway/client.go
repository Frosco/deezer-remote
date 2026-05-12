package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
)

const (
	defaultBaseURL  = "https://www.deezer.com/ajax/gw-light.php"
	defaultAPIVer   = "1.0"
	defaultInput    = "3"
	bootstrapAPITok = "null" // gw-light protocol: literal string "null" for the first call.
)

// Client is the low-level adapter for Deezer's unofficial gw-light gateway.
//
// Cookie jar is required: gw-light binds the CSRF token to the server-set
// `sid` cookie. Replacing the http.Client without preserving the jar will
// break authenticated calls with "Invalid CSRF token".
type Client struct {
	http     *http.Client
	arl      string
	baseURL  string
	apiToken string
}

// NewClient builds a Client with default transport.
func NewClient(arl string) (*Client, error) {
	return newClientWithTransport(arl, http.DefaultTransport)
}

func newClientWithTransport(arl string, rt http.RoundTripper) (*Client, error) {
	if arl == "" {
		return nil, fmt.Errorf("gateway: empty arl")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("cookiejar: %w", err)
	}
	c := &Client{
		http:     &http.Client{Transport: rt, Jar: jar},
		arl:      arl,
		baseURL:  defaultBaseURL,
		apiToken: bootstrapAPITok,
	}
	// Seed the jar with the arl cookie.
	u, _ := url.Parse(c.baseURL)
	jar.SetCookies(u, []*http.Cookie{{
		Name:   "arl",
		Value:  arl,
		Domain: "deezer.com",
		Path:   "/",
	}})
	return c, nil
}

// Call POSTs `params` to gw-light's `method` endpoint and returns the raw
// `results` JSON. The current apiToken is sent as a query parameter.
//
// Errors are classified via classifyError before returning.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal params: %w", err)
	}
	// gw-light expects null/empty body for nil params; both `{}` and `null` work.
	if string(body) == "null" {
		body = []byte("{}")
	}

	u := fmt.Sprintf(
		"%s?method=%s&input=%s&api_version=%s&api_token=%s",
		c.baseURL,
		url.QueryEscape(method),
		defaultInput,
		defaultAPIVer,
		url.QueryEscape(c.apiToken),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, classifyError("", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// gw-light response envelope: { "error": [] OR { CODE: "msg" }, "results": ... }
	var env struct {
		Error   json.RawMessage `json:"error"`
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}

	if errCode := extractErrorCode(env.Error); errCode != "" {
		return nil, fmt.Errorf("call %s: %w", method, classifyError(errCode, resp.StatusCode))
	}

	return env.Results, nil
}

// extractErrorCode returns the first key of an `error` object, or "" if the
// `error` field is missing, an empty array, or an empty object.
func extractErrorCode(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try object form: {"CODE":"message"}.
	var asObj map[string]string
	if err := json.Unmarshal(raw, &asObj); err == nil {
		for k := range asObj {
			return k
		}
		return ""
	}
	// Array form (usually empty): [].
	return ""
}
