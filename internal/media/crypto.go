package media

import (
	"crypto/md5"
	"encoding/hex"
)

// blowfishSecret is the 16-char XOR mask used by Deezer's key derivation.
// Documented in deemix/d-fi; not a placeholder.
const blowfishSecret = "g4el58wc0zvf9na1"

// KeyFromSNGID derives the 16-byte Blowfish-CBC key for a given track.
//
//	key[i] = md5_hex[i] XOR md5_hex[i+16] XOR secret[i]
//
// where md5_hex is the lowercase-hex MD5 of the SNG_ID string and secret is
// the known 16-char constant. XOR is on byte values of the ASCII chars,
// not on the bytes the hex represents.
func KeyFromSNGID(sngID string) [16]byte {
	sum := md5.Sum([]byte(sngID))
	hexed := hex.EncodeToString(sum[:])
	var k [16]byte
	for i := 0; i < 16; i++ {
		k[i] = hexed[i] ^ hexed[i+16] ^ blowfishSecret[i]
	}
	return k
}
