package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded filesystem rooted at the "dist" directory.
// Returns an error if the dist directory is not available (e.g. not built yet).
func DistFS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
