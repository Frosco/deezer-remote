// Package web exposes the embedded SPA bundle that lives under web/dist/.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the embedded SPA filesystem rooted at web/dist/.
func FS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // compile-time invariant; only fires if //go:embed misses
	}
	return sub
}
