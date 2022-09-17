package v1

import "C"
import (
	"context"
	"github.com/rancher/opni/pkg/agent/shared"
	"os"
	"path"
)

type Compressible interface {
	Compress() ([]byte, error)
	Extract() ([]byte, error)
}

// LeftJoinSlice
//
// everything in arr1 should be in arr2
// arr1 should override arr2 with the new info
func LeftJoinSlice[T comparable](arr1, arr2 []T) []T {
	result := make([]T, len(arr1))
	cache := map[T]struct{}{}
	for i, v := range arr1 {
		cache[v] = struct{}{}
		result[i] = v
	}
	for _, v := range arr2 {
		if _, ok := cache[v]; !ok {
			result = append(result, v)
		}
	}
	return result
}

type PatchInfo struct {
	Op          PatchOp
	NewRevision string
	OldRevision string
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
		for pluginPath, _ := range m.Items { //constant time so ok
			if path.Base(otherPath) == path.Base(pluginPath) {
				found = true
				break
			}
		}
		if !found {
			res.Items[path.Base(otherPath)] = PatchInfo{
				Op:          PatchOp_REMOVE,
				OldRevision: other.Items[otherPath].Metadata,
				NewRevision: "",
			}
		}
	}

	for pluginPath, metadata := range m.Items {
		found := false
		for otherPath, otherMetadata := range other.Items { // constant time so ok
			if path.Base(otherPath) == path.Base(pluginPath) {
				found = true
				if metadata.Metadata != otherMetadata.Metadata {
					res.Items[path.Base(pluginPath)] = PatchInfo{
						Op:          PatchOp_UPDATE,
						NewRevision: metadata.Metadata,
						OldRevision: otherMetadata.Metadata,
					}
				}
			}
		}
		if !found {
			res.Items[path.Base(pluginPath)] = PatchInfo{
				Op:          PatchOp_CREATE,
				NewRevision: metadata.Metadata,
				OldRevision: "",
			}
		}
	}
	return res, nil
}

func getManifestBytes(
	cache shared.PatchCache,
	compression shared.BytesCompression,
	pluginName string,
	operationInfo PatchInfo,
	ctx context.Context,
	remoteServer PluginManifestServer,
) ([]byte, error) {
	pluginDir := path.Dir(shared.PluginPathGlob())
	switch operationInfo.Op {
	case PatchOp_CREATE:
		return os.ReadFile(path.Join(pluginDir, pluginName))
	case PatchOp_UPDATE:
		if data, err := cache.Get(pluginName, operationInfo.OldRevision, operationInfo.NewRevision); err == nil {
			return data, nil
		}
		incomingResp, err := remoteServer.GetCompressedManifests(ctx, &ManifestMetadataList{
			ReqCompr: CompressionMethod_PLAIN,
			Items: map[string]*ManifestMetadata{
				pluginName: &ManifestMetadata{
					Metadata: "",
				},
			},
		})
		if err != nil {
			return nil, err // FIXME aggregate errors
		}
		incomingData := compression.MustExtract(incomingResp.Items[pluginName].GetData(), nil)
		newData, err := os.ReadFile(path.Join(pluginDir, pluginName))
		if err != nil {
			return nil, err // FIXME aggregate errors
		}
		return shared.GeneratePatch(incomingData, newData)
	default:
		return []byte{}, nil
	}
}

func GenerateManifestListFromPatchOps(
	todo *TODOList,
	cache shared.PatchCache,
	compression shared.BytesCompression,
	ctx context.Context,
	remoteServer PluginManifestServer,
) (*ManifestList, error) {

	res := &ManifestList{
		Manifests: make(map[string]*CompressedManifest),
	}
	for pluginName, operationInfo := range todo.Items {

		comprData := compression.MustCompress(getManifestBytes(cache, compression, pluginName, operationInfo, ctx, remoteServer))
		res.Manifests[pluginName] = &CompressedManifest{
			ComprMethod: CompressionMethod_PLAIN,
			DataAndInfo: &ManifestData{
				Data: comprData,
				Op:   operationInfo.Op,
			},
		}
	}
	return res, nil
}
