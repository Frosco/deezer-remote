package media

import (
	"bytes"
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"io"
	"testing"

	"golang.org/x/crypto/blowfish"
)

func TestKeyFromSNGID_KnownVector(t *testing.T) {
	// SNG_ID = "1" -> md5("1") = "c4ca4238a0b923820dcc509a6f75849b" (lowercase hex).
	// Algorithm: key[i] = md5_hex[i] XOR md5_hex[i+16] XOR secret[i] (XOR of byte values).
	got := KeyFromSNGID("1")

	// Independent derivation for the test: compute it inline.
	want := derivedKey(t, "1")
	if !bytes.Equal(got[:], want[:]) {
		t.Errorf("KeyFromSNGID(\"1\") = %x, want %x", got, want)
	}
}

func TestKeyFromSNGID_Deterministic(t *testing.T) {
	a := KeyFromSNGID("3135556")
	b := KeyFromSNGID("3135556")
	if a != b {
		t.Error("KeyFromSNGID not deterministic")
	}
}

// derivedKey is the reference implementation re-derived inside the test, so
// the test does not just compare the function against itself.
func derivedKey(t *testing.T, sngID string) [16]byte {
	t.Helper()
	sum := md5.Sum([]byte(sngID))
	hexed := hex.EncodeToString(sum[:]) // 32 lowercase hex chars
	const secret = "g4el58wc0zvf9na1"
	var k [16]byte
	for i := 0; i < 16; i++ {
		k[i] = hexed[i] ^ hexed[i+16] ^ secret[i]
	}
	return k
}

func TestDecrypt_StrideRespected(t *testing.T) {
	const blockSize = 2048
	const numBlocks = 9 // exercises three encrypted-block positions: 0, 3, 6

	key := [16]byte{}
	for i := range key {
		key[i] = byte(i + 1) // arbitrary non-zero key
	}

	// Build 9 plaintext blocks. Blocks 0/3/6 will be encrypted; rest passthrough.
	plain := make([]byte, numBlocks*blockSize)
	for i := 0; i < numBlocks*blockSize; i++ {
		plain[i] = byte(i % 251) // deterministic non-trivial pattern
	}

	// Encrypt blocks 0, 3, 6 in place to build the "encrypted stream".
	encStream := make([]byte, len(plain))
	copy(encStream, plain)

	bc, err := blowfish.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	iv := []byte{0, 1, 2, 3, 4, 5, 6, 7}
	for blk := 0; blk < numBlocks; blk++ {
		if blk%3 != 0 {
			continue
		}
		start := blk * blockSize
		mode := cipher.NewCBCEncrypter(bc, iv)
		mode.CryptBlocks(encStream[start:start+blockSize], plain[start:start+blockSize])
	}

	// Now Decrypt should recover `plain` from `encStream`.
	r := Decrypt(bytesReader(encStream), key)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(plain) {
		t.Fatalf("len = %d, want %d", len(got), len(plain))
	}
	for i := range plain {
		if got[i] != plain[i] {
			t.Fatalf("mismatch at byte %d (block %d, offset %d): got %02x want %02x", i, i/blockSize, i%blockSize, got[i], plain[i])
		}
	}
}

func TestDecrypt_PartialFinalBlockPassthrough(t *testing.T) {
	const blockSize = 2048
	key := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	// One full passthrough block (index 1, not 0) + 500 trailing bytes.
	// Encrypted positions are i%3==0, so index 1 is passthrough.
	stream := make([]byte, blockSize+500)
	for i := range stream {
		stream[i] = byte(i % 251)
	}
	// Insert one encrypted block at index 0 by pre-pending.
	// To keep this test simple, just check that bytes after position 0 pass through.
	// We use only blocks 1 + a partial — index 0 will exist and be "encrypted",
	// but we control its plaintext to be all-zeros so encryption produces ciphertext
	// that, when decrypted, yields zeros — predictable.
	allZeros := make([]byte, blockSize)
	bc, _ := blowfish.NewCipher(key[:])
	enc0 := make([]byte, blockSize)
	cipher.NewCBCEncrypter(bc, []byte{0, 1, 2, 3, 4, 5, 6, 7}).CryptBlocks(enc0, allZeros)

	encStream := append([]byte{}, enc0...)
	encStream = append(encStream, stream...)

	r := Decrypt(bytesReader(encStream), key)
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(encStream) {
		t.Fatalf("len = %d, want %d", len(got), len(encStream))
	}
	// First block should now be all zeros.
	for i := 0; i < blockSize; i++ {
		if got[i] != 0 {
			t.Fatalf("block 0 byte %d = %02x, want 0", i, got[i])
		}
	}
	// Trailing 500 bytes should be untouched (passthrough as part of the
	// partial final block).
	for i := 0; i < 500; i++ {
		want := byte((blockSize + i) % 251)
		if got[blockSize+blockSize+i] != want {
			t.Fatalf("trailing byte %d = %02x, want %02x", i, got[blockSize+blockSize+i], want)
		}
	}
}

// bytesReader wraps a []byte as an io.Reader that reads up to a chunked size
// per call, to expose buffering bugs in Decrypt.
func bytesReader(b []byte) io.Reader {
	return &chunkedReader{buf: b, chunk: 137} // arbitrary non-aligned chunk
}

type chunkedReader struct {
	buf   []byte
	pos   int
	chunk int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.buf) {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(p) {
		n = len(p)
	}
	if n > len(r.buf)-r.pos {
		n = len(r.buf) - r.pos
	}
	copy(p, r.buf[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}
