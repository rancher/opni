package v1

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-plugin"
	"go.uber.org/zap"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/rancher/opni/pkg/agent/shared"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins"
)

type FilesystemPluginSyncServer struct {
	UnsafePluginManifestServer
	Logger *zap.SugaredLogger
	Config v1beta1.PluginsSpec

	loadMetadataOnce sync.Once
	pluginMetadata   *ManifestMetadataList
	patchCache       shared.PatchCache

	inflightUploadRequestsMu sync.Mutex
	inflightUploadRequests   map[string]*time.Timer
}

func NewFilesystemPluginSyncServer(cfg v1beta1.PluginsSpec, lg *zap.SugaredLogger) *FilesystemPluginSyncServer {
	return &FilesystemPluginSyncServer{
		Config:                 cfg,
		Logger:                 lg,
		inflightUploadRequests: make(map[string]*time.Timer),
		patchCache:             shared.NewInMemoryCache(),
	}
}

func (f *FilesystemPluginSyncServer) getPluginMetadata() *ManifestMetadataList {
	f.loadMetadataOnce.Do(func() {
		md, err := GetFilesystemPlugins(f.Config, f.Logger)
		if err != nil {
			panic(err)
		}
		f.pluginMetadata = md
	})
	return f.pluginMetadata
}

//var CompressionMap = make(map[CompressionMethod]shared.BytesCompression)
//
//func init() {
//	CompressionMap[CompressionMethod_PLAIN] = &shared.NoCompression{}
//	CompressionMap[CompressionMethod_ZSTD] = &shared.ZstdCompression{}
//}

var _ PluginManifestServer = &FilesystemPluginSyncServer{}

func GetFilesystemPlugins(config v1beta1.PluginsSpec, lg *zap.SugaredLogger) (*ManifestMetadataList, error) {
	lg.Debug("plugin manifests requested")

	var matches []string
	for _, dir := range config.Dirs {
		items, err := plugin.Discover(plugins.DefaultPluginGlob, dir)
		if err == nil {
			matches = append(matches, items...)
		}
	}
	res := &ManifestMetadataList{
		Items: map[string]*ManifestMetadata{},
	}

	for _, pluginPath := range matches {
		f, err := os.Open(pluginPath)
		if err != nil {
			return nil, err
		}
		hash, _ := blake2b.New256(nil)
		if _, err := io.Copy(hash, f); err != nil {
			f.Close()
			return nil, fmt.Errorf("error reading plugin: %w", err)
		}
		sum := hex.EncodeToString(hash.Sum(nil))
		f.Close()

		//info, err := buildinfo.ReadFile(pluginPath)
		//if err != nil {
		//	return nil, err //FIXME:  aggregate errors
		//}
		//var revision *string
		//for _, s := range info.Settings {
		//	if s.Key == "vcs.revision" {
		//		revision = &s.Value
		//		break
		//	}
		//}
		//if revision == nil {
		//	revision = &shared.UnknownRevision
		//}
		//if err != nil {
		//	// do something
		//	lg.Warnf("failed to read plugin metadata for %s", pluginPath)
		//}

		res.Items[pluginPath] = &ManifestMetadata{
			Hash: sum,
			Path: pluginPath,
			//Revision: *revision,
		}
	}
	return res, nil
}

func (f *FilesystemPluginSyncServer) SendManifestsOrKnownPatch(
	ctx context.Context,
	theirManifestMetadata *ManifestMetadataList,
) (*ManifestList, error) {
	// on startup
	ourManifestMetadata := f.getPluginMetadata()
	//FIXME
	//compMethod := CompressionMethod_ZSTD
	//comp := CompressionMap[compMethod]

	ops, err := ourManifestMetadata.LeftJoinOn(theirManifestMetadata)
	if err != nil {
		return nil, err
	}
	res := &ManifestList{
		Manifests: make(map[string]*CompressedManifest),
	}
	for pluginName, op := range ops.Items {
		if data, err := f.patchCache.Get(pluginName, op.OldHash, op.NewHash); err == nil {
			res.Manifests[pluginName] = &CompressedManifest{
				//ComprMethod: compMethod,
				DataAndInfo: &ManifestData{
					Data:    data,
					OpPath:  op.TheirPath,
					Op:      op.Op,
					IsPatch: true,
					OldHash: op.OldHash,
					NewHash: op.NewHash,
				},
			}
		} else {
			data, err := os.ReadFile(op.OurPath)
			if err != nil {
				return nil, err
			}
			// if there are no pending requests for an agent to upload this patch, ask this agent to upload it
			key := f.patchCache.Key(pluginName, op.OldHash, op.NewHash)
			requestUpload := false
			f.inflightUploadRequestsMu.Lock()
			if _, ok := f.inflightUploadRequests[key]; !ok {
				requestUpload = true
				// time this out after 60 seconds
				f.inflightUploadRequests[key] = time.AfterFunc(1*time.Minute, func() {
					f.inflightUploadRequestsMu.Lock()
					defer f.inflightUploadRequestsMu.Unlock()
					f.Logger.With(
						"plugin", pluginName,
					).Warn("timed out waiting for agent to upload patch")
					delete(f.inflightUploadRequests, key)
				})
			} else {
				f.Logger.With(
					"plugin", pluginName,
				).Info("waiting on another agent to upload a patch for this plugin")
			}
			f.inflightUploadRequestsMu.Unlock()

			if requestUpload {
				f.Logger.With(
					"plugin", pluginName,
				).Info("requesting agent to compute and upload patch for this plugin")
			} else {
				f.Logger.With(
					"plugin", pluginName,
				).Info("not requesting patch upload")
			}

			res.Manifests[pluginName] = &CompressedManifest{
				//ComprMethod: compMethod,
				DataAndInfo: &ManifestData{
					Data:               data,
					OpPath:             op.TheirPath,
					Op:                 op.Op,
					IsPatch:            false,
					RequestPatchUpload: requestUpload,
					OldHash:            op.OldHash,
					NewHash:            op.NewHash,
				},
			}
		}
	}
	return res, nil
}

func (f *FilesystemPluginSyncServer) GetPluginManifests(ctx context.Context, _ *emptypb.Empty) (*ManifestMetadataList, error) {
	lg := f.Logger.With("method", "GetPluginManifests")

	return GetFilesystemPlugins(f.Config, lg)
}

//func (f *FilesystemPluginSyncServer) GetCompressedManifests(
//	ctx context.Context,
//	list *ManifestMetadataList,
//) (*CompressedManifests, error) {
//
//	method, ok := CompressionMap[list.ReqCompr]
//	if !ok {
//		method = &shared.NoCompression{}
//	}
//	if err := list.Validate(); err != nil {
//		return nil, err
//	}
//	res := &CompressedManifests{
//		Items: map[string]*ManifestData{},
//	}
//	for pluginPath, _ := range list.Items {
//		//fixme support compression
//		raw, err := os.ReadFile(pluginPath)
//		if err != nil {
//			return nil, err // FIXME : aggregate errors
//		}
//		res.Items[pluginPath] = &ManifestData{
//			Data: util.Must(method.Compress(raw)),
//		}
//	}
//	return res, nil
//}

func (f *FilesystemPluginSyncServer) UploadPatch(ctx context.Context, spec *PatchSpec) (*emptypb.Empty, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	f.Logger.With(
		"pluginName", spec.PluginName,
		"oldHash", spec.OldHash,
		"newHash", spec.NewHash,
	).Info("patch received")

	f.inflightUploadRequestsMu.Lock()
	defer f.inflightUploadRequestsMu.Unlock()

	f.patchCache.Put(spec.PluginName, spec.OldHash, spec.NewHash, spec.Patch)

	key := f.patchCache.Key(spec.PluginName, spec.OldHash, spec.NewHash)
	if t, ok := f.inflightUploadRequests[key]; ok {
		if t.Stop() {
			delete(f.inflightUploadRequests, key)
		}
	}
	return &emptypb.Empty{}, nil
}

func PatchWith(
	ctx context.Context,
	config v1beta1.PluginsSpec,
	receivedManifests *ManifestList,
	lg *zap.SugaredLogger,
	gatewaySyncClient PluginManifestClient,
) (chan struct{}, error) {
	if len(config.Dirs) == 0 {
		return nil, fmt.Errorf("no plugin directories configured")
	}
	done := make(chan struct{})

	var backgroundTasks sync.WaitGroup

	group := errgroup.Group{}
	for pluginBaseName, v := range receivedManifests.Manifests {
		pluginBaseName := pluginBaseName
		v := v
		group.Go(func() error {

			//comprMethod := v.GetComprMethod()
			//comp, ok := CompressionMap[comprMethod]
			//if !ok {
			//	comp = &shared.NoCompression{}
			//}
			info := v.GetDataAndInfo()
			fullPluginPath := info.GetOpPath()
			if fullPluginPath == "" {
				fullPluginPath = filepath.Join(config.Dirs[0], pluginBaseName)
			}
			lg := lg.With("plugin", fullPluginPath)

			receivedData := info.GetData()
			//var decompressedData []byte
			//if d := info.GetData(); len(d) > 0 {
			//	var err error
			//	decompressedData, err = comp.Extract(info.GetData())
			//	if err != nil {
			//		return nil, err
			//	}
			//}

			switch info.GetOp() {
			case PatchOp_CREATE:
				lg.Info("writing new plugin")
				err := os.WriteFile(fullPluginPath, receivedData, 0755)
				if err != nil {
					return err
				}
			case PatchOp_UPDATE:
				if info.GetIsPatch() { // patch received from gateway
					lg.Info("patching plugin")
					existingData, err := os.ReadFile(fullPluginPath)
					if err != nil {
						return err
					}
					patchResult, err := shared.ApplyPatch(existingData, receivedData)
					if err != nil {
						return err
					}
					err = os.WriteFile(fullPluginPath, patchResult, 0755)
					if err != nil {
						return err
					}
				} else { // whole file received from gateway
					lg.Info("updating plugin")
					existingData, err := os.ReadFile(fullPluginPath)
					if err != nil {
						return err
					}

					err = os.WriteFile(fullPluginPath, receivedData, 0755)
					if err != nil {
						return err
					}
					if info.RequestPatchUpload {
						lg.Info("gateway requested patch upload for plugin")
						oldHash := v.GetDataAndInfo().GetOldHash()
						newHash := v.GetDataAndInfo().GetNewHash()
						backgroundTasks.Add(1)
						go func() {
							defer backgroundTasks.Done()
							lg.Info("computing patch")
							patch, err := shared.GeneratePatch(existingData, receivedData)
							if err != nil {
								lg.With(
									zap.Error(err),
								).Error("failed to generate patch")
								return
							}
							lg.Info("uploading computed patch")
							_, err = gatewaySyncClient.UploadPatch(ctx, &PatchSpec{
								PluginName: pluginBaseName,
								OldHash:    oldHash,
								NewHash:    newHash,
								Patch:      patch,
							})
							if err != nil {
								lg.With(
									zap.Error(err),
								).Error("failed to upload patch")
							}
						}()
					}
				}
			case PatchOp_REMOVE:
				err := os.Remove(fullPluginPath)
				if err != nil {
					return err
				}
			default:
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	go func() {
		backgroundTasks.Wait()
		close(done)
		lg.Debug("background patching tasks complete")
	}()

	return done, nil
}

//
//func (f *FilesystemPluginSyncServer) PatchManifests(ctx context.Context, req *ManifestList) (*ManifestMetadataList, error) {
//	//lg := f.Logger.With("method", "PatchManifests")
//	return nil, nil
//	//return PatchWith(f.Config, req, lg)
//}
