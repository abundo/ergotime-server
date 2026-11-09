// Package web embeds the production Vue SPA built into dist/.
//
// Build the frontend before compiling the server so assets are present:
//
//	make frontend   # or: make release
package web

import (
	"embed"
	"io/fs"
)

// distEmbed holds the Vite build output (index.html + assets/).
//
//go:embed all:dist
var distEmbed embed.FS

// DistFS returns the SPA root (contents of dist/), suitable for http.FS.
func DistFS() (fs.FS, error) {
	return fs.Sub(distEmbed, "dist")
}
