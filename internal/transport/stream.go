package transport

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/niref/deezer-remote/internal/media"
)

// Resolver is the subset of media.Resolver the handler needs.
type Resolver interface {
	Resolve(ctx context.Context, trackID string) (media.ResolvedTrack, error)
}

// StreamHandler serves decrypted MP3 bytes over /stream/{id}.
type StreamHandler struct {
	res   Resolver
	httpc *http.Client
}

// NewStreamHandler wires a handler. httpc is the CDN-facing client; pass
// http.DefaultClient unless tests need otherwise.
func NewStreamHandler(res Resolver, httpc *http.Client) *StreamHandler {
	return &StreamHandler{res: res, httpc: httpc}
}

// Register attaches the handler at /stream/{id}. Caller wraps with RequireToken.
func (h *StreamHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /stream/{id}", h.serve)
}

func (h *StreamHandler) serve(w http.ResponseWriter, r *http.Request) {
	trackID := r.PathValue("id")
	rt, err := h.res.Resolve(r.Context(), trackID)
	if err != nil {
		http.Error(w, "not_available: "+err.Error(), http.StatusNotFound)
		return
	}

	start, end, partial, ok := parseRangeHeader(r.Header.Get("Range"), rt.Size)
	if !ok {
		http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	key := media.KeyFromSNGID(rt.SngID)
	body, err := media.NewRangeStream(r.Context(), h.httpc, media.RangeStreamInput{
		URL:   rt.URL,
		Key:   key,
		Start: start,
		End:   end,
	})
	if err != nil {
		http.Error(w, "stream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer body.Close()

	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, rt.Size))
		w.WriteHeader(http.StatusPartialContent)
	}
	_, _ = io.Copy(w, body)
}

// parseRangeHeader extracts a single byte range. Supports "bytes=N-M",
// "bytes=N-" (open-ended), and missing / empty Range (full file).
// Returns ok=false on syntactically invalid input.
func parseRangeHeader(h string, total int64) (start, end int64, partial, ok bool) {
	if h == "" {
		return 0, total - 1, false, true
	}
	if !strings.HasPrefix(h, "bytes=") {
		return 0, 0, false, false
	}
	spec := strings.TrimPrefix(h, "bytes=")
	dash := strings.IndexByte(spec, '-')
	if dash < 0 {
		return 0, 0, false, false
	}
	s, e := spec[:dash], spec[dash+1:]
	var err error
	if s == "" {
		// Suffix range "bytes=-N": last N bytes.
		var n int64
		n, err = strconv.ParseInt(e, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false, false
		}
		start = total - n
		if start < 0 {
			start = 0
		}
		end = total - 1
	} else {
		start, err = strconv.ParseInt(s, 10, 64)
		if err != nil || start < 0 {
			return 0, 0, false, false
		}
		if e == "" {
			end = total - 1
		} else {
			end, err = strconv.ParseInt(e, 10, 64)
			if err != nil || end < start {
				return 0, 0, false, false
			}
		}
	}
	if end >= total {
		end = total - 1
	}
	if start >= total {
		return 0, 0, false, false
	}
	return start, end, true, true
}
