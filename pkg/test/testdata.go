package test

import (
	"embed"
	"io/fs"
	"path/filepath"
)

//go:embed testdata
var TestDataFS embed.FS

func TestData(filename string) []byte {
	data, err := fs.ReadFile(TestDataFS, filepath.Join("testdata", filename))
	if err != nil {
		panic(err)
	}
	return data
}
