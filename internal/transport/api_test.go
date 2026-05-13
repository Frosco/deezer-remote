package transport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/niref/deezer-remote/internal/gateway"
)

// fakeGW is a tiny double for the gateway methods APIHandlers needs.
type fakeGW struct {
	search   func(ctx context.Context, q string, n int) (*gateway.SearchResult, error)
	track    func(ctx context.Context, id string) (*gateway.TrackData, error)
	album    func(ctx context.Context, id string) (*gateway.Album, error)
	playlist func(ctx context.Context, id string) (*gateway.Playlist, error)
}

func (f fakeGW) Search(ctx context.Context, q string, n int) (*gateway.SearchResult, error) {
	return f.search(ctx, q, n)
}
func (f fakeGW) SongGetData(ctx context.Context, id string) (*gateway.TrackData, error) {
	return f.track(ctx, id)
}
func (f fakeGW) Album(ctx context.Context, id string) (*gateway.Album, error) {
	return f.album(ctx, id)
}
func (f fakeGW) Playlist(ctx context.Context, id string) (*gateway.Playlist, error) {
	return f.playlist(ctx, id)
}

func TestAPI_Search_PassesQueryAndShapesResponse(t *testing.T) {
	var seenQuery string
	gw := fakeGW{
		search: func(ctx context.Context, q string, n int) (*gateway.SearchResult, error) {
			seenQuery = q
			return &gateway.SearchResult{
				Tracks: []gateway.TrackSummary{{ID: "1", Title: "T", Artist: "A", Album: "Z", CoverMD5: "md5x", DurationS: 200}},
			}, nil
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/search?q=daft", nil)
	mux.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code = %d body = %s", rr.Code, rr.Body)
	}
	if seenQuery != "daft" {
		t.Errorf("seenQuery = %q", seenQuery)
	}
	var body struct {
		Tracks []struct {
			ID       string `json:"id"`
			CoverURL string `json:"cover_url"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Tracks) != 1 || body.Tracks[0].ID != "1" {
		t.Errorf("body = %+v", body)
	}
	if !strings.Contains(body.Tracks[0].CoverURL, "md5x") {
		t.Errorf("CoverURL = %q (expected to contain md5x)", body.Tracks[0].CoverURL)
	}
}

func TestAPI_Search_400OnMissingQ(t *testing.T) {
	mux := http.NewServeMux()
	NewAPIHandlers(fakeGW{}).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/search", nil))
	if rr.Code != 400 {
		t.Errorf("code = %d", rr.Code)
	}
}

func TestAPI_Album(t *testing.T) {
	gw := fakeGW{
		album: func(ctx context.Context, id string) (*gateway.Album, error) {
			if id != "10" {
				t.Errorf("id = %q", id)
			}
			return &gateway.Album{
				Header: gateway.AlbumSummary{ID: "10", Title: "RAM", Artist: "Daft Punk", CoverMD5: "pic", TrackCount: 2},
				Tracks: []gateway.TrackSummary{
					{ID: "1", Title: "A", Artist: "Daft Punk", Album: "RAM", CoverMD5: "pic", DurationS: 200},
					{ID: "2", Title: "B", Artist: "Daft Punk", Album: "RAM", CoverMD5: "pic", DurationS: 220},
				},
			}, nil
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/album/10", nil))
	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body)
	}
}

func TestAPI_Track_404OnNotFound(t *testing.T) {
	gw := fakeGW{
		track: func(ctx context.Context, id string) (*gateway.TrackData, error) {
			return nil, gateway.ErrNotFound
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/track/999", nil))
	if rr.Code != 404 {
		t.Errorf("code = %d", rr.Code)
	}
}

func TestAPI_AuthErrors_Are401(t *testing.T) {
	gw := fakeGW{
		search: func(ctx context.Context, q string, n int) (*gateway.SearchResult, error) {
			return nil, errors.Join(gateway.ErrAuthFailed)
		},
	}
	mux := http.NewServeMux()
	NewAPIHandlers(gw).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/api/search?q=daft", nil))
	if rr.Code != 401 {
		t.Errorf("code = %d", rr.Code)
	}
}
