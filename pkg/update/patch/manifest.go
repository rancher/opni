package patch

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sync"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/urn"
	"github.com/samber/lo"
	"github.com/spf13/afero"
	"golang.org/x/crypto/blake2b"
)

func LeftJoinOn(gateway, agent *controlv1.UpdateManifest) *controlv1.PatchList {
	res := &controlv1.PatchList{}
	ourPlugins := map[string]*controlv1.UpdateManifestEntry{}
	// if a plugin gets renamed we need to look up hashes
	ourDigests := map[string]struct{}{}
	theirPlugins := map[string]*controlv1.UpdateManifestEntry{}
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
				Package:   ours.GetId(),
				Op:        controlv1.PatchOp_Create,
				Path:      ours.GetPath(),
				OldDigest: "",
				NewDigest: ours.GetDigest(),
			})
		} else {
			// both sides have the plugin
			if ours.GetDigest() != theirs.GetDigest() {
				// the hashes are different
				res.Items = append(res.Items, &controlv1.PatchSpec{
					Package:   ours.GetId(),
					Op:        controlv1.PatchOp_Update,
					Path:      ours.GetPath(),
					OldDigest: theirs.GetDigest(),
					NewDigest: ours.GetDigest(),
				})
			} else if ours.GetPath() != theirs.GetPath() {
				// a plugin was renamed but the hash is the same
				res.Items = append(res.Items, &controlv1.PatchSpec{
					Package:   ours.GetId(),
					Op:        controlv1.PatchOp_Rename,
					Data:      []byte(ours.GetPath()),
					Path:      theirs.GetPath(),
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
					Package:   theirs.GetId(),
					Op:        controlv1.PatchOp_Remove,
					Path:      theirs.GetPath(),
					OldDigest: theirs.GetDigest(),
					NewDigest: "",
				})
			}
		}
	}
	res.Sort()
	return res
}

func GetFilesystemPlugins(dc plugins.DiscoveryConfig) (*controlv1.PluginArchive, error) {
	if dc.Fs == nil {
		dc.Fs = afero.NewOsFs()
	}
	matches := dc.Discover()
	res := &controlv1.PluginArchive{
		Items: make([]*controlv1.PluginArchiveEntry, len(matches)),
	}
	lg := dc.Logger
	if lg == nil {
		lg = logger.NewNop()
	}
	lg.Debug(fmt.Sprintf("found %d plugins", len(matches)))
	var wg sync.WaitGroup
	for i, md := range matches {
		i, md := i, md
		wg.Add(1)
		go func() {
			defer wg.Done()
			lg := lg.With("path", md.BinaryPath)
			f, err := dc.Fs.Open(md.BinaryPath)
			if err != nil {
				lg.With(
					zap.Error(err),
				).Error("failed to read plugin, skipping")
				return
			}
			defer f.Close()

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
			opniURN := urn.NewOpniURN(urn.Plugin, UpdateStrategy, md.Module)
			res.Items[i] = &controlv1.PluginArchiveEntry{
				Metadata: &controlv1.UpdateManifestEntry{
					Package: opniURN.String(),
					Path:    filepath.Base(md.BinaryPath),
					Digest:  sum,
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
		lg.Warn(fmt.Sprintf("%d plugins failed to load", numFailed))
	}

	res.Items = lo.WithoutEmpty(res.Items)
	res.Sort()
	lg.With(
		PluginsDir, len(res.Items),
	).Debug("loaded plugin manifest")
	return res, nil
}
