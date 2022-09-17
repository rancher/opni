package shared

import (
	"bytes"
	"fmt"
	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
	"github.com/gabstv/go-bsdiff/pkg/bspatch"
	"regexp"
	"text/template"
)

var UnknownRevision = "vcs.revision.unknown"
var PluginPathTemplate = template.Must(template.New("").Parse("bin/plugins/plugin_{{.MatchExpr}}"))

type BytesCompression interface {
	Compress([]byte) ([]byte, error)
	MustCompress([]byte, error) []byte
	Extract([]byte) ([]byte, error)
	MustExtract([]byte, error) []byte
}

var _ BytesCompression = &NoCompression{}

type NoCompression struct{}

func (c *NoCompression) Compress(data []byte) ([]byte, error) {
	return data, nil
}

func (c *NoCompression) MustCompress(data []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return data
}

func (c *NoCompression) Extract(data []byte) ([]byte, error) {
	return data, nil
}

func (c *NoCompression) MustExtract(data []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return data
}

type PatchCache interface {
	Get(pluginName, oldRevision, newRevision string) ([]byte, error)
	Put(pluginName, oldRevision, newRevision string, patch []byte) error
}

var _ PatchCache = &NoCache{}

type NoCache struct{}

func (c *NoCache) Get(pluginName, oldRevision, newRevision string) ([]byte, error) {
	return nil, fmt.Errorf("no cache available")
}
func (c *NoCache) Put(pluginName, oldRevision, newRevision string, patch []byte) error {
	return nil
}

func GeneratePatch(outdatedBytes, newestBytes []byte) ([]byte, error) {
	// BSDIFF4 patch
	patch, err := bsdiff.Bytes(outdatedBytes, newestBytes)
	if err != nil {
		return []byte{}, err
	}
	return patch, nil
}

func ApplyPatch(outdatedBytes, patch []byte) ([]byte, error) {
	newBytes, err := bspatch.Bytes(outdatedBytes, patch)
	if err != nil {
		return []byte{}, err
	}
	return newBytes, nil
}

func PluginPathRegex() *regexp.Regexp {
	var b bytes.Buffer
	err := PluginPathTemplate.Execute(&b, map[string]string{
		"MatchExpr": ".*",
	})
	if err != nil {
		panic(err)
	}
	return regexp.MustCompile(b.String())
}

func PluginPathGlob() string {
	var b bytes.Buffer
	err := PluginPathTemplate.Execute(&b, map[string]string{
		"MatchExpr": "*",
	})
	if err != nil {
		panic(err)
	}
	return b.String()
}
