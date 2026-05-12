package gateway

import (
	"context"
	"encoding/json"
	"errors"
)

// userDataEnvelope is the subset of deezer.getUserData we care about.
type userDataEnvelope struct {
	CheckForm string `json:"checkForm"`
	User      struct {
		UserID  int `json:"USER_ID"`
		Options struct {
			LicenseToken string `json:"license_token"`
		} `json:"OPTIONS"`
	} `json:"USER"`
}

// refreshCSRF calls deezer.getUserData with api_token=null and updates
// c.apiToken from the response's checkForm field. USER_ID==0 is treated as
// ErrAuthFailed (gw-light's signal that the arl is invalid).
func (c *Client) refreshCSRF(ctx context.Context) error {
	prev := c.apiToken
	c.apiToken = bootstrapAPITok
	raw, err := c.Call(ctx, "deezer.getUserData", nil)
	if err != nil {
		c.apiToken = prev
		return err
	}
	var ud userDataEnvelope
	if err := json.Unmarshal(raw, &ud); err != nil {
		c.apiToken = prev
		return err
	}
	if ud.User.UserID == 0 {
		c.apiToken = prev
		return ErrAuthFailed
	}
	c.apiToken = ud.CheckForm
	return nil
}

// callWithCSRF is the public path for authenticated gw-light methods.
// Ensures apiToken is set (bootstrap if not), and retries once on
// ErrCSRFExpired by refreshing the token.
func (c *Client) callWithCSRF(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if c.apiToken == "" || c.apiToken == bootstrapAPITok {
		if err := c.refreshCSRF(ctx); err != nil {
			return nil, err
		}
	}

	raw, err := c.Call(ctx, method, params)
	if err == nil {
		return raw, nil
	}
	if !errors.Is(err, ErrCSRFExpired) {
		return nil, err
	}

	// One retry: refresh and try again.
	if rerr := c.refreshCSRF(ctx); rerr != nil {
		return nil, rerr
	}
	return c.Call(ctx, method, params)
}
