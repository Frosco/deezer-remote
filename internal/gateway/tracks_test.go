package gateway

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestGetUserData_ReturnsLicenseToken(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42,"OPTIONS":{"license_token":"LIC-TOK"}},"checkForm":"CFTOK"}}`), nil
	})
	c, _ := newClientWithTransport("ARL", rt)
	ud, err := c.GetUserData(context.Background())
	if err != nil {
		t.Fatalf("GetUserData: %v", err)
	}
	if ud.LicenseToken != "LIC-TOK" {
		t.Errorf("LicenseToken = %q, want LIC-TOK", ud.LicenseToken)
	}
	if ud.UserID != 42 {
		t.Errorf("UserID = %d, want 42", ud.UserID)
	}
}

func TestGetUserData_PropagatesAuthFailed(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":0,"OPTIONS":{"license_token":""}},"checkForm":""}}`), nil
	})
	c, _ := newClientWithTransport("BAD-ARL", rt)
	_, err := c.GetUserData(context.Background())
	if !errors.Is(err, ErrAuthFailed) {
		t.Errorf("err = %v, want ErrAuthFailed", err)
	}
}

func TestSongGetData_ReturnsTrackInfo(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case isUserDataCall(req):
			return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42,"OPTIONS":{"license_token":"LT"}},"checkForm":"CF"}}`), nil
		default:
			return mkResp(200, `{"error":[],"results":{"SNG_ID":"3135556","TRACK_TOKEN":"TT-XYZ","MD5_ORIGIN":"abc","MEDIA_VERSION":"7","SNG_TITLE":"Get Lucky","ART_NAME":"Daft Punk"}}`), nil
		}
	})
	c, _ := newClientWithTransport("ARL", rt)
	td, err := c.SongGetData(context.Background(), "3135556")
	if err != nil {
		t.Fatalf("SongGetData: %v", err)
	}
	if td.SngID != "3135556" {
		t.Errorf("SngID = %q, want 3135556", td.SngID)
	}
	if td.TrackToken != "TT-XYZ" {
		t.Errorf("TrackToken = %q", td.TrackToken)
	}
	if td.Title != "Get Lucky" {
		t.Errorf("Title = %q", td.Title)
	}
	if td.Artist != "Daft Punk" {
		t.Errorf("Artist = %q", td.Artist)
	}
}

func TestSongGetData_FlexStringHandlesBareNumber(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case isUserDataCall(req):
			return mkResp(200, `{"error":[],"results":{"USER":{"USER_ID":42,"OPTIONS":{"license_token":"LT"}},"checkForm":"CF"}}`), nil
		default:
			// SNG_ID as a bare number (no quotes).
			return mkResp(200, `{"error":[],"results":{"SNG_ID":3135556,"TRACK_TOKEN":"TT","MD5_ORIGIN":"a","MEDIA_VERSION":"1","SNG_TITLE":"X","ART_NAME":"Y"}}`), nil
		}
	})
	c, _ := newClientWithTransport("ARL", rt)
	td, err := c.SongGetData(context.Background(), "3135556")
	if err != nil {
		t.Fatalf("SongGetData: %v", err)
	}
	if td.SngID != "3135556" {
		t.Errorf("SngID = %q, want 3135556", td.SngID)
	}
}

// isUserDataCall reports whether req is the deezer.getUserData bootstrap.
func isUserDataCall(req *http.Request) bool {
	return req.URL.Query().Get("method") == "deezer.getUserData"
}
