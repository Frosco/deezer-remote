package transport

import (
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/niref/deezer-remote/internal/media"
	"golang.org/x/crypto/blowfish"
)

const testBlockSize = 2048
const testEncStride = 3

func buildEncryptedFile(t *testing.T, key [16]byte, total int) ([]byte, []byte) {
	t.Helper()
	plain := make([]byte, total)
	for i := range plain {
		plain[i] = byte(i % 251)
	}
	enc := make([]byte, total)
	bc, err := blowfish.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	for off, idx := 0, 0; off < total; off, idx = off+testBlockSize, idx+1 {
		end := off + testBlockSize
		if end > total {
			copy(enc[off:], plain[off:])
			break
		}
		if idx%testEncStride == 0 {
			cipher.NewCBCEncrypter(bc, iv).CryptBlocks(enc[off:end], plain[off:end])
		} else {
			copy(enc[off:end], plain[off:end])
		}
	}
	return plain, enc
}

func serveBytes(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ra := r.Header.Get("Range")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		if ra == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(200)
			_, _ = w.Write(body)
			return
		}
		var start, end int64
		if _, err := fmt.Sscanf(ra, "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", 416)
			return
		}
		if end >= int64(len(body)) {
			end = int64(len(body)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", end-start+1))
		w.WriteHeader(206)
		_, _ = w.Write(body[start : end+1])
	}))
	t.Cleanup(srv.Close)
	return srv
}

// fakeResolver lets stream tests inject a pre-fabricated ResolvedTrack.
type fakeResolver struct {
	fn func(ctx context.Context, trackID string) (media.ResolvedTrack, error)
}

func (f fakeResolver) Resolve(ctx context.Context, id string) (media.ResolvedTrack, error) {
	return f.fn(ctx, id)
}

func TestStream_ServesFullFile(t *testing.T) {
	key := media.KeyFromSNGID("42")
	total := 6*testBlockSize + 500
	plain, enc := buildEncryptedFile(t, key, total)
	cdn := serveBytes(t, enc)

	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{
			TrackID: "42", SngID: "42", URL: cdn.URL,
			Format: media.FormatMP3_320, Size: int64(total), ExpiryAt: time.Now().Add(time.Hour),
		}, nil
	}}

	mux := http.NewServeMux()
	NewStreamHandler(res, http.DefaultClient).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/stream/42", nil))

	if rr.Code != 200 {
		t.Fatalf("code = %d body = %s", rr.Code, rr.Body)
	}
	if rr.Header().Get("Content-Length") != strconv.Itoa(total) {
		t.Errorf("Content-Length = %q want %d", rr.Header().Get("Content-Length"), total)
	}
	if rr.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges = %q", rr.Header().Get("Accept-Ranges"))
	}
	if rr.Header().Get("Content-Type") != "audio/mpeg" {
		t.Errorf("Content-Type = %q", rr.Header().Get("Content-Type"))
	}
	got, _ := io.ReadAll(rr.Body)
	if len(got) != len(plain) {
		t.Errorf("len(got)=%d, len(plain)=%d", len(got), len(plain))
	}
}

func TestStream_ServesPartialOnRange(t *testing.T) {
	key := media.KeyFromSNGID("42")
	total := 6*testBlockSize + 100
	plain, enc := buildEncryptedFile(t, key, total)
	cdn := serveBytes(t, enc)

	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{
			TrackID: "42", SngID: "42", URL: cdn.URL,
			Format: media.FormatMP3_320, Size: int64(total), ExpiryAt: time.Now().Add(time.Hour),
		}, nil
	}}

	mux := http.NewServeMux()
	NewStreamHandler(res, http.DefaultClient).Register(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stream/42", nil)
	req.Header.Set("Range", "bytes=4096-8191")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("code = %d body = %s", rr.Code, rr.Body)
	}
	if rr.Header().Get("Content-Length") != "4096" {
		t.Errorf("Content-Length = %q", rr.Header().Get("Content-Length"))
	}
	if rr.Header().Get("Content-Range") != fmt.Sprintf("bytes 4096-8191/%d", total) {
		t.Errorf("Content-Range = %q", rr.Header().Get("Content-Range"))
	}
	got, _ := io.ReadAll(rr.Body)
	if string(got) != string(plain[4096:8192]) {
		t.Error("decrypted bytes mismatch")
	}
}

func TestStream_NotAvailable_Is404(t *testing.T) {
	res := fakeResolver{fn: func(ctx context.Context, id string) (media.ResolvedTrack, error) {
		return media.ResolvedTrack{}, errors.New("get_url: track unavailable")
	}}
	mux := http.NewServeMux()
	NewStreamHandler(res, http.DefaultClient).Register(mux)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/stream/42", nil))
	if rr.Code != 404 {
		t.Errorf("code = %d", rr.Code)
	}
}
