package patch

import (
	"bytes"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashicorp/go-plugin"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/crypto/blake2b"
)

func LeftJoinOn(gateway, agent *controlv1.PluginManifest) *controlv1.PatchList {
	res := &controlv1.PatchList{}
	ourPlugins := map[string]*controlv1.PluginManifestEntry{}
	// if a plugin gets renamed we need to look up hashes
	ourDigests := map[string]struct{}{}
	theirPlugins := map[string]*controlv1.PluginManifestEntry{}
	theirDigests := map[string]struct{}{}
	for _, v := range gateway.Items {
		ourDigests[v.GetDigest()] = struct{}{}
		ourPlugins[v.GetId()] = v
	}
	for _, v := range agent.Items {
		theirPlugins[v.GetId()] = v
		theirDigests[v.GetDigest()] = struct{}{}
	}
	for _, ours := range gateway.Items {
		if theirs, ok := theirPlugins[ours.GetId()]; !ok {
			// we have a plugin that they don't have
			res.Items = append(res.Items, &controlv1.PatchSpec{
				Module:    ours.GetId(),
				Op:        controlv1.PatchOp_Create,
				Filename:  ours.GetFilename(),
				OldDigest: "",
				NewDigest: ours.GetDigest(),
			})
		} else {
			// both sides have the plugin
			if ours.GetDigest() != theirs.GetDigest() {
				// the hashes are different
				res.Items = append(res.Items, &controlv1.PatchSpec{
					Module:    ours.GetId(),
					Op:        controlv1.PatchOp_Update,
					Filename:  ours.GetFilename(),
					OldDigest: theirs.GetDigest(),
					NewDigest: ours.GetDigest(),
				})
			} else if ours.GetFilename() != theirs.GetFilename() {
				// a plugin was renamed but the hash is the same
				res.Items = append(res.Items, &controlv1.PatchSpec{
					Module:    ours.GetId(),
					Op:        controlv1.PatchOp_Rename,
					Data:      []byte(ours.GetFilename()),
					Filename:  theirs.GetFilename(),
					OldDigest: theirs.GetDigest(),
					NewDigest: ours.GetDigest(),
				})
			}
		}
	}
	for _, theirs := range agent.Items {
		if _, ok := ourPlugins[theirs.GetId()]; !ok {
			if _, ok := ourDigests[theirs.GetDigest()]; !ok {
				// they have a plugin that we don't have
				res.Items = append(res.Items, &controlv1.PatchSpec{
					Module:    theirs.GetId(),
					Op:        controlv1.PatchOp_Remove,
					Filename:  theirs.GetFilename(),
					OldDigest: theirs.GetDigest(),
					NewDigest: "",
				})
			}
		}
	}
	res.Sort()
	return res
}

func GetFilesystemPlugins(config v1beta1.PluginsSpec, lg *zap.SugaredLogger) (*controlv1.PluginArchive, error) {
	matches, err := plugin.Discover(plugins.DefaultPluginGlob, config.Dir)
	if err != nil {
		return nil, err
	}
	res := &controlv1.PluginArchive{
		Items: make([]*controlv1.PluginArchiveEntry, len(matches)),
	}
	lg.Debugf("found %d plugins", len(matches))
	var wg sync.WaitGroup
	for i, pluginPath := range matches {
		i, pluginPath := i, pluginPath
		wg.Add(1)
		go func() {
			defer wg.Done()
			lg := lg.With("path", pluginPath)
			f, err := os.Open(pluginPath)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("failed to read plugin, skipping")
				return
			}
			defer f.Close()
			pluginMetadata, err := meta.ReadFile(f)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("failed to read plugin metadata, skipping")
				return
			}

			fileInfo, _ := f.Stat()
			fileSize := fileInfo.Size()

			hash, _ := blake2b.New256(nil)
			contents := bytes.NewBuffer(make([]byte, 0, fileSize))

			if _, err := io.Copy(io.MultiWriter(hash, contents), f); err != nil {
				lg.With(
					zap.Error(err),
				).Error("failed to read plugin, skipping")
				return
			}
			sum := hex.EncodeToString(hash.Sum(nil))
			res.Items[i] = &controlv1.PluginArchiveEntry{
				Metadata: &controlv1.PluginManifestEntry{
					Module:   pluginMetadata.Module,
					Filename: filepath.Base(pluginPath),
					Digest:   sum,
				},
				Data: contents.Bytes(),
			}
		}()
	}
	wg.Wait()
	// count and remove nil entries
	var numFailed int
	for i := 0; i < len(res.Items); i++ {
		if res.Items[i] == nil {
			numFailed++
		}
	}
	if numFailed > 0 {
		lg.Warnf("%d plugins failed to load", numFailed)
	}

	res.Items = lo.WithoutEmpty(res.Items)
	res.Sort()
	lg.With(
		"plugins", len(res.Items),
	).Debug("loaded plugin manifest")
	return res, nil
}
