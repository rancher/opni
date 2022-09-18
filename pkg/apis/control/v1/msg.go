package v1

import (
	"path"
)

type PatchInfo struct {
	Op        PatchOp
	OurPath   string
	TheirPath string
	NewHash   string
	OldHash   string
}

type TODOList struct {
	// pluginPath to  operation
	Items map[string]PatchInfo
}

func (m *ManifestMetadataList) LeftJoinOn(other *ManifestMetadataList) (*TODOList, error) {
	res := &TODOList{
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
