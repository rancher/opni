package testdata

import (
	"embed"
	"io/fs"
	"path"
)

//go:embed testdata

var TestDataFS embed.FS

func TestData(filename string) []byte {
	data, err := fs.ReadFile(TestDataFS, path.Join("testdata", filename))
	if err != nil {
		panic(err)
	}
	return data
}
