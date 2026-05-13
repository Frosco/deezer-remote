package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrCDNNoRange means the CDN ignored a Range request. The streaming proxy
// must reject this rather than serve mis-aligned bytes.
var ErrCDNNoRange = errors.New("media: CDN did not honour Range request")

// RangeStreamInput parameterises NewRangeStream.
type RangeStreamInput struct {
	URL   string
	Key   [16]byte
	Start int64 // inclusive
	End   int64 // inclusive; must be >= Start
}

// NewRangeStream issues a Range-aware GET to the encrypted CDN URL and
// returns a reader that yields plaintext bytes for the [Start, End] range.
// The reader fetches the smallest 2048-byte-aligned superset of the requested
// range, then trims the leading slack bytes.
func NewRangeStream(ctx context.Context, hc *http.Client, in RangeStreamInput) (io.ReadCloser, error) {
	if in.End < in.Start {
		return nil, fmt.Errorf("range stream: End (%d) < Start (%d)", in.End, in.Start)
	}

	// Align Start down to a block boundary; End stays inclusive.
	alignedStart := (in.Start / blockSize) * blockSize
	startBlockIdx := int(alignedStart / blockSize)
	leadingSlack := in.Start - alignedStart
	wantLen := in.End - in.Start + 1

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, in.URL, nil)
	if err != nil {
		return nil, err
	}
	// Full-file range request: omit the upper bound so a CDN that knows the
	// total size can respond with the entire tail.
	if in.Start == 0 && in.End >= veryLarge {
		// No Range header — request the whole file.
	} else {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", alignedStart, in.End))
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cdn get: %w", err)
	}
	wantRange := req.Header.Get("Range") != ""
	if wantRange && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, ErrCDNNoRange
	}
	if !wantRange && resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("cdn get: http %d", resp.StatusCode)
	}

	dec := newDecryptReaderAtBlock(resp.Body, in.Key, startBlockIdx)
	// Discard the leading slack and cap the output to wantLen bytes.
	if leadingSlack > 0 {
		if _, err := io.CopyN(io.Discard, dec, leadingSlack); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("trim leading slack: %w", err)
		}
	}
	limited := io.LimitReader(dec, wantLen)
	return readCloser{Reader: limited, closer: resp.Body}, nil
}

// veryLarge is a sentinel "no end" used by full-file callers.
const veryLarge int64 = (1 << 62)

type readCloser struct {
	io.Reader
	closer io.Closer
}

func (rc readCloser) Close() error { return rc.closer.Close() }
