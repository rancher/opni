package util

import (
	"bytes"
	"io"
)

func ReadString(r io.Reader) string {
	var b bytes.Buffer
	b.ReadFrom(r)
	return b.String()
}
