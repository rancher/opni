package patch

import (
	"bytes"
	"io"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
	"github.com/gabstv/go-bsdiff/pkg/bspatch"
)

const (
	UpdateStrategy = "binary"
)

type BinaryPatcher interface {
	GeneratePatch(old io.Reader, new io.Reader, patchOut io.Writer) error
	ApplyPatch(old io.Reader, patch io.Reader, newOut io.Writer) error
	CheckFormat(reader io.ReaderAt) bool
}

type BsdiffPatcher struct{}

func (BsdiffPatcher) GeneratePatch(old io.Reader, new io.Reader, patchOut io.Writer) (err error) {
	return bsdiff.Reader(old, new, patchOut)
}

func (BsdiffPatcher) ApplyPatch(old io.Reader, patch io.Reader, newOut io.Writer) (err error) {
	return bspatch.Reader(old, newOut, patch)
}

func (BsdiffPatcher) CheckFormat(reader io.ReaderAt) bool {
	header := []byte("BSDIFF40")
	buf := make([]byte, len(header))
	if _, err := reader.ReadAt(buf, 0); err == nil {
		return bytes.Equal(buf, header)
	}
	return false
}

var allPatchEngines = []BinaryPatcher{
	BsdiffPatcher{},
}

func NewPatcherFromFormat(reader io.ReaderAt) (BinaryPatcher, bool) {
	for _, p := range allPatchEngines {
		if p.CheckFormat(reader) {
			return p, true
		}
	}
	return nil, false
}
