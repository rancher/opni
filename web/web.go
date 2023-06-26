package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var DistFS embed.FS

func EmbeddedAssetsAvailable(fs fs.FS) bool {
	f, err := fs.Open("dist")
	if err != nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}
	return true
}
