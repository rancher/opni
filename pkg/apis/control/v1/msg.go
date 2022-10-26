package v1

import (
	"encoding/hex"
	"path"

	"github.com/samber/lo"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/exp/slices"
)

type PatchInfo struct {
	Op        PatchOp
	OurPath   string
	TheirPath string
	NewHash   string
	OldHash   string
}

type PatchList struct {
	// pluginPath to  operation
	Items map[string]PatchInfo
}

func (m *ManifestMetadataList) LeftJoinOn(other *ManifestMetadataList) (*PatchList, error) {
	res := &PatchList{
		Items: make(map[string]PatchInfo),
	}
	for otherPath, _ := range other.Items {
		found := false
		foundPath := ""
		for pluginPath, _ := range m.Items { //constant time so ok
			if path.Base(otherPath) == path.Base(pluginPath) {
				foundPath = pluginPath
				found = true
				break
			}
		}
		if !found {
			res.Items[path.Base(otherPath)] = PatchInfo{
				OurPath:   foundPath,
				TheirPath: otherPath,
				Op:        PatchOp_REMOVE,
				OldHash:   other.Items[otherPath].Hash,
				NewHash:   "",
			}
		}
	}

	for pluginPath, metadata := range m.Items {
		found := false
		for otherPath, otherMetadata := range other.Items { // constant time so ok
			if path.Base(otherPath) == path.Base(pluginPath) {
				found = true
				if metadata.Hash != otherMetadata.Hash {
					res.Items[path.Base(pluginPath)] = PatchInfo{
						OurPath:   pluginPath,
						TheirPath: otherPath,
						Op:        PatchOp_UPDATE,
						NewHash:   metadata.Hash,
						OldHash:   otherMetadata.Hash,
					}
				}
			}
		}
		if !found {
			res.Items[path.Base(pluginPath)] = PatchInfo{
				OurPath:   pluginPath,
				TheirPath: "",
				Op:        PatchOp_CREATE,
				NewHash:   metadata.Hash,
				OldHash:   "",
			}
		}
	}
	return res, nil
}

const ManifestDigestKey = "manifest-digest"

// Returns a hash of the manifest metadata list. This can be used to compare
// manifests between the gateway and agent.
func (m *ManifestMetadataList) Digest() string {
	hash, _ := blake2b.New256(nil)
	keys := lo.Keys(m.Items)
	slices.SortFunc(keys, func(a, b string) bool {
		return path.Base(a) < path.Base(b)
	})
	for _, key := range keys {
		item, ok := m.Items[key]
		if ok {
			hash.Write([]byte(path.Base(key)))
			hash.Write([]byte(item.Hash))
			hash.Write([]byte(item.Revision))
		}
	}
	return hex.EncodeToString(hash.Sum(nil))
}
