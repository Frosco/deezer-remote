// Package gateway is a thin client for Deezer's unofficial gw-light gateway.
package gateway

import "errors"

var (
	// ErrCSRFExpired means apiToken must be refreshed and the call retried.
	ErrCSRFExpired = errors.New("gateway: CSRF token expired")
	// ErrAuthFailed means the arl is invalid or revoked.
	ErrAuthFailed = errors.New("gateway: auth failed; refresh arl in config")
	// ErrNotFound means the requested resource doesn't exist.
	ErrNotFound = errors.New("gateway: not found")
	// ErrRateLimited means gw-light QUOTA_ERROR or HTTP 429.
	ErrRateLimited = errors.New("gateway: rate limited")
	// ErrServerError means HTTP 5xx from gw-light.
	ErrServerError = errors.New("gateway: server error")
	// ErrUnknown means gw-light returned an error string we don't recognise.
	ErrUnknown = errors.New("gateway: unknown error")
)

// classifyError maps a gw-light error string and HTTP status onto a sentinel.
// Returns nil when there is no error.
func classifyError(body string, status int) error {
	if status >= 500 {
		return ErrServerError
	}
	if status == 429 {
		return ErrRateLimited
	}
	switch body {
	case "":
		if status == 200 {
			return nil
		}
		return ErrUnknown
	case "CSRF_TOKEN_INVALID", "VALID_TOKEN_REQUIRED":
		return ErrCSRFExpired
	case "NEED_USER_AUTH_REQUIRED", "USER_AUTH_REQUIRED":
		return ErrAuthFailed
	case "DATA_ERROR":
		return ErrNotFound
	case "QUOTA_ERROR":
		return ErrRateLimited
	default:
		return ErrUnknown
	}
}
