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

// Assumes the remote server is the gateway
//func getManifestBytes(
//	cache shared.PatchCache,
//	compressionMethod CompressionMethod,
//	operationInfo PatchInfo,
//	ctx context.Context,
//	gatewayServer PluginManifestClient,
//) ([]byte, error) {
//	comp, ok := CompressionMap[compressionMethod]
//	if !ok {
//		comp = &shared.NoCompression{}
//	}
//	switch operationInfo.Op {
//	case PatchOp_CREATE:
//		data, err := os.ReadFile(operationInfo.OurPath)
//		if err != nil {
//			return nil, err
//		}
//		return comp.Compress(data)
//	case PatchOp_UPDATE:
//		if data, err := cache.Get(operationInfo.OurPath, operationInfo.OldHash, operationInfo.NewHash); err == nil {
//			return data, nil
//		}
//		incomingResp, err := gatewayServer.GetCompressedManifests(ctx, &ManifestMetadataList{
//			ReqCompr: compressionMethod,
//			Items: map[string]*ManifestMetadata{
//				operationInfo.TheirPath: &ManifestMetadata{},
//			},
//		})
//		if err != nil {
//			return nil, err // FIXME aggregate errors
//		}
//		comp := CompressionMap[compressionMethod]
//		incomingData, err := comp.Extract(incomingResp.Items[operationInfo.TheirPath].GetData())
//		if err != nil {
//			return nil, err
//		}
//		oldData, err := os.ReadFile(operationInfo.OurPath)
//		if err != nil {
//			return nil, err // FIXME aggregate errors
//		}
//		return shared.GeneratePatch(oldData, incomingData)
//	default:
//		return []byte{}, nil
//	}
//}

// assumes the remote server is the gateway
//func GenerateManifestListFromPatchOps(
//	todo *TODOList,
//	cache shared.PatchCache,
//	comprType CompressionMethod,
//	ctx context.Context,
//	remoteServer PluginManifestClient,
//) (*ManifestList, error) {
//	comp, ok := CompressionMap[comprType]
//	if !ok {
//		comp = &shared.NoCompression{}
//	}
//	res := &ManifestList{
//		Manifests: make(map[string]*CompressedManifest),
//	}
//	for pluginName, operationInfo := range todo.Items {
//		bytes, err := getManifestBytes(cache, comprType, operationInfo, ctx, remoteServer)
//		if err != nil {
//			return nil, err
//		}
//		//comprData, err := comp.Compress(bytes)
//		//if err != nil {
//		//	return nil, err
//		//}
//		res.Manifests[pluginName] = &CompressedManifest{
//			ComprMethod: comprType,
//			DataAndInfo: &ManifestData{
//				Data:   comprData,
//				Op:     operationInfo.Op,
//				OpPath: operationInfo.TheirPath,
//			},
//		}
//	}
//	return res, nil
//}
