package gateway

import (
	"context"
	"encoding/json"
	"strconv"
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

// flexString decodes a JSON field that gw-light returns sometimes as a
// quoted string and sometimes as a bare number.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(string(b))
	return nil
}

// TrackData is the subset of song.getData we expose.
type TrackData struct {
	SngID           string
	TrackToken      string
	MD5Origin       string
	MediaVersion    string
	Title           string
	Artist          string
	Album           string
	CoverMD5        string // ALB_PICTURE; build URL via image CDN
	DurationS       int
	FileSizeMP3_320 int64
	FileSizeMP3_128 int64
}

type songGetDataEnvelope struct {
	SngID           flexString `json:"SNG_ID"`
	TrackToken      string     `json:"TRACK_TOKEN"`
	MD5Origin       string     `json:"MD5_ORIGIN"`
	MediaVersion    flexString `json:"MEDIA_VERSION"`
	Title           string     `json:"SNG_TITLE"`
	Artist          string     `json:"ART_NAME"`
	Album           string     `json:"ALB_TITLE"`
	CoverMD5        string     `json:"ALB_PICTURE"`
	Duration        flexString `json:"DURATION"`
	FileSizeMP3_320 flexString `json:"FILESIZE_MP3_320"`
	FileSizeMP3_128 flexString `json:"FILESIZE_MP3_128"`
}

// SongGetData fetches metadata for one track via gw-light song.getData.
func (c *Client) SongGetData(ctx context.Context, trackID string) (*TrackData, error) {
	raw, err := c.callWithCSRF(ctx, "song.getData", map[string]any{"SNG_ID": trackID})
	if err != nil {
		return nil, err
	}
	var env songGetDataEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if env.SngID == "" {
		return nil, ErrNotFound
	}
	return &TrackData{
		SngID:           string(env.SngID),
		TrackToken:      env.TrackToken,
		MD5Origin:       env.MD5Origin,
		MediaVersion:    string(env.MediaVersion),
		Title:           env.Title,
		Artist:          env.Artist,
		Album:           env.Album,
		CoverMD5:        env.CoverMD5,
		DurationS:       atoiOrZero(string(env.Duration)),
		FileSizeMP3_320: atoi64OrZero(string(env.FileSizeMP3_320)),
		FileSizeMP3_128: atoi64OrZero(string(env.FileSizeMP3_128)),
	}, nil
}

func atoiOrZero(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

func atoi64OrZero(s string) int64 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
