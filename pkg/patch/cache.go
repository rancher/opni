package patch

import (
	"fmt"
	"io"

	"github.com/gabstv/go-bsdiff/pkg/bsdiff"
	"github.com/gabstv/go-bsdiff/pkg/bspatch"
	lru "github.com/hashicorp/golang-lru"
	"github.com/klauspost/compress/zstd"
	"google.golang.org/grpc/encoding"

	"github.com/rancher/opni/pkg/util"
)

type ZstdCompressor struct{}

func (c ZstdCompressor) Compress(w io.Writer) (io.WriteCloser, error) {
	return zstd.NewWriter(w)
}

func (c ZstdCompressor) Decompress(r io.Reader) (io.Reader, error) {
	return zstd.NewReader(r)
}

func (c ZstdCompressor) Name() string {
	return "zstd"
}

func init() {
	encoding.RegisterCompressor(ZstdCompressor{})
}

type PatchCache interface {
	Get(pluginName, oldRevision, newRevision string) ([]byte, error)
	Put(pluginName, oldRevision, newRevision string, patch []byte) error
	Key(pluginName, oldRevision, newRevision string) string
}

type InMemoryCache struct {
	cache *lru.Cache
}

func (c *InMemoryCache) Key(pluginName, oldRevision, newRevision string) string {
	return fmt.Sprintf("%s-%s-to-%s", pluginName, oldRevision, newRevision)
}

func NewInMemoryCache() PatchCache {
	return &InMemoryCache{
		cache: util.Must(lru.New(32)),
	}
}

func (c *InMemoryCache) Get(pluginName, oldRevision, newRevision string) ([]byte, error) {
	if v, ok := c.cache.Get(c.Key(pluginName, oldRevision, newRevision)); ok {
		return v.([]byte), nil
	}
	return nil, fmt.Errorf("not found")
}

func (c *InMemoryCache) Put(pluginName, oldRevision, newRevision string, patch []byte) error {
	c.cache.Add(c.Key(pluginName, oldRevision, newRevision), patch)
	return nil
}

// writes in place the result to the patchf io.Writer
func GenerateIOStreamPatch(oldBin io.Reader, newBin io.Reader, patchf io.Writer) error {
	return bsdiff.Reader(oldBin, newBin, patchf)
}

// writes in place the result to the newBin io.Writer
func ApplyIOStreamPatch(oldBin io.Reader, patchf io.Reader, newBin io.Writer) error {
	return bspatch.Reader(oldBin, newBin, patchf)
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
