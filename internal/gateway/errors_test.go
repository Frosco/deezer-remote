package gateway

import (
	"errors"
	"testing"
)

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		status int
		want   error
	}{
		{"csrf invalid", "CSRF_TOKEN_INVALID", 200, ErrCSRFExpired},
		{"valid token required", "VALID_TOKEN_REQUIRED", 200, ErrCSRFExpired},
		{"need user auth", "NEED_USER_AUTH_REQUIRED", 200, ErrAuthFailed},
		{"user auth required", "USER_AUTH_REQUIRED", 200, ErrAuthFailed},
		{"data error", "DATA_ERROR", 200, ErrNotFound},
		{"quota error", "QUOTA_ERROR", 200, ErrRateLimited},
		{"http 429", "", 429, ErrRateLimited},
		{"http 500", "", 500, ErrServerError},
		{"http 502", "", 502, ErrServerError},
		{"unknown body", "WEIRD_NEW_ERROR_CODE", 200, ErrUnknown},
		{"http 200 clean", "", 200, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyError(tc.body, tc.status)
			if tc.want == nil {
				if got != nil {
					t.Errorf("classifyError(%q, %d) = %v, want nil", tc.body, tc.status, got)
				}
				return
			}
			if !errors.Is(got, tc.want) {
				t.Errorf("classifyError(%q, %d) = %v, want %v", tc.body, tc.status, got, tc.want)
			}
		})
	}
}
