package media

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"testing"
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
