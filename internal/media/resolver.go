package media

import (
	"context"
	"fmt"
	"time"
)

// TrackInfo is the subset of song.getData the resolver needs. The gateway
// package provides a TrackData that satisfies this shape; we declare a local
// type so internal/media stays independent of internal/gateway (strict
// layering: media must not import gateway).
type TrackInfo struct {
	SngID           string
	TrackToken      string
	FileSizeMP3_320 int64
	FileSizeMP3_128 int64
}

// TrackFetcher abstracts the two outbound calls the resolver makes. The
// transport-layer wiring code provides an adapter over gateway.Client and
// media.URLClient.
type TrackFetcher interface {
	SongGetData(ctx context.Context, trackID string) (TrackInfo, error)
	GetURL(ctx context.Context, req URLRequest) (*URLResult, error)
}

// Resolver turns a track_id into a ResolvedTrack, using a Cache to skip the
// outbound calls when a fresh entry is available.
type Resolver struct {
	f            TrackFetcher
	licenseToken string
	formats      []Format
	cache        *Cache
}

// NewResolver wires a resolver. formats is the preference order passed to
// media.getUrl (typically [MP3_320, MP3_128]).
func NewResolver(f TrackFetcher, licenseToken string, formats []Format, cache *Cache) *Resolver {
	return &Resolver{f: f, licenseToken: licenseToken, formats: formats, cache: cache}
}

// Resolve returns a fresh, ready-to-stream ResolvedTrack.
func (r *Resolver) Resolve(ctx context.Context, trackID string) (ResolvedTrack, error) {
	if hit, ok := r.cache.Get(trackID); ok {
		return hit, nil
	}
	ti, err := r.f.SongGetData(ctx, trackID)
	if err != nil {
		return ResolvedTrack{}, fmt.Errorf("song.getData(%s): %w", trackID, err)
	}
	res, err := r.f.GetURL(ctx, URLRequest{
		LicenseToken: r.licenseToken,
		TrackToken:   ti.TrackToken,
		Formats:      r.formats,
	})
	if err != nil {
		return ResolvedTrack{}, fmt.Errorf("media.getUrl(%s): %w", trackID, err)
	}
	size := sizeForFormat(ti, res.Format)
	out := ResolvedTrack{
		TrackID:  trackID,
		SngID:    ti.SngID,
		URL:      res.URL,
		Format:   res.Format,
		Size:     size,
		ExpiryAt: time.Unix(res.Expiry, 0),
	}
	r.cache.Put(out)
	return out, nil
}

func sizeForFormat(ti TrackInfo, f Format) int64 {
	switch f {
	case FormatMP3_320:
		return ti.FileSizeMP3_320
	case FormatMP3_128:
		return ti.FileSizeMP3_128
	default:
		return 0
	}
}
