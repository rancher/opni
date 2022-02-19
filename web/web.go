package web

import (
	"embed"
)

//go:embed dist/*
var DistFS embed.FS

func EmbeddedAssetsAvailable() bool {
	f, err := DistFS.Open("dist/_nuxt")
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
