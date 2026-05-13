package media

import (
	"bytes"
	"context"
	"crypto/cipher"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/crypto/blowfish"
)

// makeEncryptedFixture builds totalBytes worth of input where every 3rd
// 2048-byte block is encrypted with the given key, mirroring Deezer's scheme.
// Returns (plaintext, ciphertext).
func makeEncryptedFixture(t *testing.T, key [16]byte, totalBytes int) ([]byte, []byte) {
	t.Helper()
	plain := make([]byte, totalBytes)
	for i := range plain {
		plain[i] = byte(i % 251) // arbitrary deterministic pattern
	}
	cipherOut := make([]byte, totalBytes)
	bc, err := blowfish.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	for off, idx := 0, 0; off < totalBytes; off, idx = off+blockSize, idx+1 {
		end := off + blockSize
		if end > totalBytes {
			// Final partial: passthrough.
			copy(cipherOut[off:], plain[off:])
			break
		}
		if idx%encryptionStride == 0 {
			cipher.NewCBCEncrypter(bc, blowfishIV).CryptBlocks(cipherOut[off:end], plain[off:end])
		} else {
			copy(cipherOut[off:end], plain[off:end])
		}
	}
	return plain, cipherOut
}

func TestRangeStream_FullFile(t *testing.T) {
	key := KeyFromSNGID("42")
	plain, cipherBytes := makeEncryptedFixture(t, key, 6*blockSize+500)

	cdn := serveFixture(t, cipherBytes)
	rc, err := NewRangeStream(context.Background(), http.DefaultClient, RangeStreamInput{
		URL:   cdn.URL,
		Key:   key,
		Start: 0,
		End:   int64(len(plain)) - 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plain) {
		t.Errorf("decrypted bytes do not match plaintext (got len=%d want=%d)", len(got), len(plain))
	}
}

func TestRangeStream_AlignedSubrange(t *testing.T) {
	key := KeyFromSNGID("42")
	plain, cipherBytes := makeEncryptedFixture(t, key, 6*blockSize)
	cdn := serveFixture(t, cipherBytes)

	// Block-aligned range: start at block 3 (offset 6144).
	start := int64(3 * blockSize)
	end := int64(5*blockSize) - 1
	rc, err := NewRangeStream(context.Background(), http.DefaultClient, RangeStreamInput{
		URL: cdn.URL, Key: key, Start: start, End: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	want := plain[start : end+1]
	if !bytes.Equal(got, want) {
		t.Errorf("aligned subrange mismatch: got len=%d want=%d", len(got), len(want))
	}
}

func TestRangeStream_UnalignedSubrange(t *testing.T) {
	key := KeyFromSNGID("42")
	plain, cipherBytes := makeEncryptedFixture(t, key, 6*blockSize+500)
	cdn := serveFixture(t, cipherBytes)

	// Unaligned: start 100 bytes into block 4.
	start := int64(4*blockSize + 100)
	end := int64(5*blockSize + 200)
	rc, err := NewRangeStream(context.Background(), http.DefaultClient, RangeStreamInput{
		URL: cdn.URL, Key: key, Start: start, End: end,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	want := plain[start : end+1]
	if !bytes.Equal(got, want) {
		t.Errorf("unaligned subrange mismatch: got len=%d want=%d", len(got), len(want))
	}
}

func TestRangeStream_CDNNonPartialIsError(t *testing.T) {
	// Server returns 200 instead of 206 — our reader should reject that for a
	// requested subrange, because we cannot trust the block alignment.
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader("nope")),
			Header:     make(http.Header),
		}, nil
	})
	hc := &http.Client{Transport: rt}
	_, err := NewRangeStream(context.Background(), hc, RangeStreamInput{
		URL:   "http://cdn",
		Key:   [16]byte{},
		Start: 4096,
		End:   8191,
	})
	if err == nil || !errors.Is(err, ErrCDNNoRange) {
		t.Errorf("err = %v, want ErrCDNNoRange", err)
	}
}

// serveFixture serves cipherBytes via httptest, honouring HTTP Range requests
// the way Deezer's CDN does (200 if no Range, 206 if Range).
func serveFixture(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		ra := r.Header.Get("Range")
		if ra == "" {
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(200)
			_, _ = w.Write(body)
			return
		}
		// We only support "bytes=N-M".
		var start, end int64
		if _, err := fmt.Sscanf(ra, "bytes=%d-%d", &start, &end); err != nil {
			http.Error(w, "bad range", 416)
			return
		}
		if end >= int64(len(body)) {
			end = int64(len(body)) - 1
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(body)))
		w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
		w.WriteHeader(206)
		_, _ = w.Write(body[start : end+1])
	}))
	t.Cleanup(srv.Close)
	return srv
}
