package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireToken_AcceptsHeader(t *testing.T) {
	called := false
	h := RequireToken("SECRET")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))

	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Header.Set("Authorization", "Bearer SECRET")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if !called || rr.Code != 200 {
		t.Errorf("called=%v code=%d", called, rr.Code)
	}
}

func TestRequireToken_AcceptsQueryParam(t *testing.T) {
	called := false
	h := RequireToken("SECRET")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/stream/42?t=SECRET", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called || rr.Code != 200 {
		t.Errorf("called=%v code=%d", called, rr.Code)
	}
}

func TestRequireToken_Rejects(t *testing.T) {
	cases := []struct {
		name string
		mk   func() *http.Request
	}{
		{"missing", func() *http.Request { return httptest.NewRequest("GET", "/api/x", nil) }},
		{"wrong header", func() *http.Request {
			r := httptest.NewRequest("GET", "/api/x", nil)
			r.Header.Set("Authorization", "Bearer WRONG")
			return r
		}},
		{"wrong query", func() *http.Request {
			return httptest.NewRequest("GET", "/stream/42?t=WRONG", nil)
		}},
		{"basic auth", func() *http.Request {
			r := httptest.NewRequest("GET", "/api/x", nil)
			r.Header.Set("Authorization", "Basic Zm9vOmJhcg==")
			return r
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := RequireToken("SECRET")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("inner handler must not be called")
			}))
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, tc.mk())
			if rr.Code != 401 {
				t.Errorf("code = %d, want 401", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "unauthorized") {
				t.Errorf("body = %q", rr.Body.String())
			}
		})
	}
}
