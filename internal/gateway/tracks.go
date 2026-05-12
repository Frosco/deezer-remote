package gateway

import (
	"context"
	"encoding/json"
)

// UserData is the subset of deezer.getUserData we expose.
type UserData struct {
	UserID       int
	LicenseToken string
}

// GetUserData refreshes the CSRF token and returns the parsed user data.
// USER_ID==0 surfaces as ErrAuthFailed.
func (c *Client) GetUserData(ctx context.Context) (*UserData, error) {
	if err := c.refreshCSRF(ctx); err != nil {
		return nil, err
	}
	// After a successful refresh, do one more call to read fields, because
	// refreshCSRF discards the body. (Alternative: have refreshCSRF return
	// the parsed envelope. For Phase 1 we may refactor; for the spike it's
	// fine to make two calls — the second hits gw-light with a fresh token.)
	raw, err := c.Call(ctx, "deezer.getUserData", nil)
	if err != nil {
		return nil, err
	}
	var ud userDataEnvelope
	if err := json.Unmarshal(raw, &ud); err != nil {
		return nil, err
	}
	if ud.User.UserID == 0 {
		return nil, ErrAuthFailed
	}
	return &UserData{
		UserID:       ud.User.UserID,
		LicenseToken: ud.User.Options.LicenseToken,
	}, nil
}
