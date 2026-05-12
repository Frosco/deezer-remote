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
