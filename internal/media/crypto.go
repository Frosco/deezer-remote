package media

import (
	"crypto/cipher"
	"crypto/md5"
	"encoding/hex"
	"io"

	"golang.org/x/crypto/blowfish"
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

const (
	blockSize        = 2048
	encryptionStride = 3 // every 3rd block is encrypted
)

var blowfishIV = []byte{0, 1, 2, 3, 4, 5, 6, 7}

// Decrypt returns a reader that consumes Deezer's partially-encrypted byte
// stream and emits the plaintext audio bytes.
//
// Layout: input is split into 2048-byte blocks. Blocks at index 0, 3, 6, ...
// are Blowfish-CBC ciphertexts (key from KeyFromSNGID, IV 0x0001020304050607).
// All other blocks are passthrough. A final partial block (< 2048 bytes) is
// always passthrough.
func Decrypt(r io.Reader, key [16]byte) io.Reader {
	return &decryptReader{
		src: r,
		key: key,
	}
}

type decryptReader struct {
	src      io.Reader
	key      [16]byte
	bc       cipher.Block
	bcOnce   bool
	buf      []byte // bytes ready to be emitted to the consumer
	blockIdx int    // index of the next block to process
	srcEOF   bool   // src returned io.EOF on its last full block read
}

func (d *decryptReader) Read(p []byte) (int, error) {
	if len(d.buf) == 0 && !d.srcEOF {
		if err := d.fillNextBlock(); err != nil && len(d.buf) == 0 {
			return 0, err
		}
	}
	if len(d.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

// fillNextBlock pulls one block (up to 2048 bytes) from src, processes it,
// and appends the result to d.buf.
func (d *decryptReader) fillNextBlock() error {
	blk := make([]byte, blockSize)
	n, err := io.ReadFull(d.src, blk)
	switch {
	case err == nil:
		// Full block.
		out, perr := d.processBlock(blk[:n], true)
		if perr != nil {
			return perr
		}
		d.buf = append(d.buf, out...)
		d.blockIdx++
		return nil
	case err == io.ErrUnexpectedEOF || err == io.EOF:
		// Partial final block — always passthrough per the Deezer scheme.
		d.srcEOF = true
		if n > 0 {
			d.buf = append(d.buf, blk[:n]...)
		}
		return io.EOF
	default:
		return err
	}
}

// processBlock returns the output bytes for one full input block. If the
// current block is at an encrypted-stride position, decrypt; else passthrough.
func (d *decryptReader) processBlock(in []byte, full bool) ([]byte, error) {
	if !full || d.blockIdx%encryptionStride != 0 {
		// Passthrough — copy so callers see independent bytes.
		out := make([]byte, len(in))
		copy(out, in)
		return out, nil
	}
	if !d.bcOnce {
		bc, err := blowfish.NewCipher(d.key[:])
		if err != nil {
			return nil, err
		}
		d.bc = bc
		d.bcOnce = true
	}
	out := make([]byte, len(in))
	cipher.NewCBCDecrypter(d.bc, blowfishIV).CryptBlocks(out, in)
	return out, nil
}
