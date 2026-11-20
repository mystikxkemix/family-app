// Package webui embeds the website (built from api/web with Vite into
// internal/webui/dist) so the API binary serves it on its own.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// Files returns the built website. Before the first `npm run build`, it is
// empty (only .gitkeep), and the server shows a short "not built" page.
func Files() fs.FS {
	dist, err := fs.Sub(files, "dist")
	if err != nil {
		panic(err)
	}
	return dist
}
