package media

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeTrackFetcher implements the resolver's dependencies via closures.
type fakeTrackFetcher struct {
	songGetData func(ctx context.Context, trackID string) (TrackInfo, error)
	getURL      func(ctx context.Context, req URLRequest) (*URLResult, error)
}

func (f fakeTrackFetcher) SongGetData(ctx context.Context, trackID string) (TrackInfo, error) {
	return f.songGetData(ctx, trackID)
}
func (f fakeTrackFetcher) GetURL(ctx context.Context, req URLRequest) (*URLResult, error) {
	return f.getURL(ctx, req)
}

func TestResolver_Resolve_FetchesAndCaches(t *testing.T) {
	songCalls, urlCalls := 0, 0
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			songCalls++
			return TrackInfo{SngID: "42", TrackToken: "TT", FileSizeMP3_320: 9_000_000, FileSizeMP3_128: 3_000_000}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			urlCalls++
			return &URLResult{URL: "https://cdn/x", Format: FormatMP3_320, Expiry: time.Now().Add(20 * time.Hour).Unix()}, nil
		},
	}
	r := NewResolver(f, "LIC", []Format{FormatMP3_320, FormatMP3_128}, NewCache(30*time.Minute))

	rt1, err := r.Resolve(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if rt1.Format != FormatMP3_320 || rt1.Size != 9_000_000 {
		t.Errorf("first resolve = %+v", rt1)
	}
	rt2, err := r.Resolve(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if rt2.URL != rt1.URL {
		t.Errorf("cache miss on 2nd resolve")
	}
	if songCalls != 1 || urlCalls != 1 {
		t.Errorf("expected 1 song, 1 url; got song=%d url=%d", songCalls, urlCalls)
	}
}

func TestResolver_PicksSizeMatchingFormat(t *testing.T) {
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			return TrackInfo{SngID: "42", TrackToken: "TT", FileSizeMP3_320: 9_000_000, FileSizeMP3_128: 3_000_000}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			return &URLResult{URL: "u", Format: FormatMP3_128, Expiry: time.Now().Add(20 * time.Hour).Unix()}, nil
		},
	}
	r := NewResolver(f, "LIC", []Format{FormatMP3_320, FormatMP3_128}, NewCache(30*time.Minute))
	got, err := r.Resolve(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if got.Size != 3_000_000 {
		t.Errorf("Size = %d, want FileSizeMP3_128", got.Size)
	}
}

func TestResolver_RefetchesAfterEvict(t *testing.T) {
	calls := 0
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			return TrackInfo{SngID: "42", TrackToken: "TT", FileSizeMP3_320: 1, FileSizeMP3_128: 1}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			calls++
			return &URLResult{URL: "u", Format: FormatMP3_320, Expiry: time.Now().Add(20 * time.Hour).Unix()}, nil
		},
	}
	cache := NewCache(30 * time.Minute)
	r := NewResolver(f, "LIC", []Format{FormatMP3_320}, cache)
	if _, err := r.Resolve(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	cache.Evict("42")
	if _, err := r.Resolve(context.Background(), "42"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("expected 2 getURL calls after evict; got %d", calls)
	}
}

func TestResolver_PropagatesNotAvailable(t *testing.T) {
	f := fakeTrackFetcher{
		songGetData: func(ctx context.Context, trackID string) (TrackInfo, error) {
			return TrackInfo{SngID: "42", TrackToken: "TT"}, nil
		},
		getURL: func(ctx context.Context, req URLRequest) (*URLResult, error) {
			return nil, errors.New("get_url: track unavailable (code 2002: blocked)")
		},
	}
	r := NewResolver(f, "LIC", []Format{FormatMP3_320}, NewCache(time.Minute))
	if _, err := r.Resolve(context.Background(), "42"); err == nil {
		t.Error("expected error to propagate")
	}
}
