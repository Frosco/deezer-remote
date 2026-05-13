// Package qrterm renders QR codes to stdout / stderr using mdp/qrterminal.
package qrterm

import (
	"io"

	"github.com/mdp/qrterminal/v3"
)

// Render writes a QR code for content to w using compact half-block runes.
// Suitable for terminals supporting Unicode; falls back gracefully on
// monospaced ASCII fonts.
func Render(w io.Writer, content string) {
	cfg := qrterminal.Config{
		Level:          qrterminal.M,
		Writer:         w,
		HalfBlocks:     true,
		BlackChar:      qrterminal.BLACK_BLACK,
		WhiteChar:      qrterminal.WHITE_WHITE,
		WhiteBlackChar: qrterminal.WHITE_BLACK,
		BlackWhiteChar: qrterminal.BLACK_WHITE,
		QuietZone:      1,
	}
	qrterminal.GenerateWithConfig(content, cfg)
}
