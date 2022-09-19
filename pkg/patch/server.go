package patch

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
	"github.com/rancher/opni/pkg/agent/shared"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	v1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/plugins"
	"go.uber.org/zap"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ controlv1.PluginManifestServer = &FilesystemPluginSyncServer{}

type FilesystemPluginSyncServer struct {
	v1.UnsafePluginManifestServer
	Logger *zap.SugaredLogger
	Config v1beta1.PluginsSpec

	loadMetadataOnce sync.Once
	pluginMetadata   *v1.ManifestMetadataList
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

func (f *FilesystemPluginSyncServer) getPluginMetadata() *v1.ManifestMetadataList {
	f.loadMetadataOnce.Do(func() {
		md, err := GetFilesystemPlugins(f.Config, f.Logger)
		if err != nil {
			panic(err)
		}
		f.pluginMetadata = md
	})
	return f.pluginMetadata
}

func GetFilesystemPlugins(config v1beta1.PluginsSpec, lg *zap.SugaredLogger) (*v1.ManifestMetadataList, error) {
	lg.Debug("plugin manifests requested")

	var matches []string
	for _, dir := range config.Dirs {
		items, err := plugin.Discover(plugins.DefaultPluginGlob, dir)
		if err == nil {
			matches = append(matches, items...)
		}
	}
	res := &v1.ManifestMetadataList{
		Items: map[string]*v1.ManifestMetadata{},
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
		res.Items[pluginPath] = &v1.ManifestMetadata{
			Hash: sum,
			Path: pluginPath,
			//Revision: *revision,
		}
	}
	return res, nil
}

func (f *FilesystemPluginSyncServer) SendManifestsOrKnownPatch(
	ctx context.Context,
	theirManifestMetadata *v1.ManifestMetadataList,
) (*v1.ManifestList, error) {
	// on startup
	ourManifestMetadata := f.getPluginMetadata()

	ops, err := ourManifestMetadata.LeftJoinOn(theirManifestMetadata)
	if err != nil {
		return nil, err
	}
	res := &v1.ManifestList{
		Manifests: make(map[string]*v1.CompressedManifest),
	}
	for pluginName, op := range ops.Items {
		if data, err := f.patchCache.Get(pluginName, op.OldHash, op.NewHash); err == nil {
			res.Manifests[pluginName] = &v1.CompressedManifest{
				//ComprMethod: compMethod,
				DataAndInfo: &v1.ManifestData{
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

			res.Manifests[pluginName] = &v1.CompressedManifest{
				//ComprMethod: compMethod,
				DataAndInfo: &v1.ManifestData{
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

func (f *FilesystemPluginSyncServer) GetPluginManifests(ctx context.Context, _ *emptypb.Empty) (*v1.ManifestMetadataList, error) {
	lg := f.Logger.With("method", "GetPluginManifests")

	return GetFilesystemPlugins(f.Config, lg)
}

func (f *FilesystemPluginSyncServer) UploadPatch(ctx context.Context, spec *v1.PatchSpec) (*emptypb.Empty, error) {
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

func (f *FilesystemPluginSyncServer) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		id := cluster.StreamAuthorizedID(stream.Context())

		// for now, plugin manifest validation is voluntary, but this may change in the future
		md, ok := metadata.FromIncomingContext(stream.Context())
		if ok {
			values := md.Get(controlv1.ManifestDigestKey)
			if len(values) > 0 {
				digest := values[0]
				if f.getPluginMetadata().Digest() != digest {
					f.Logger.With(
						"id", id,
					).Info("agent plugins are out of date; requesting update")
					return status.Errorf(codes.FailedPrecondition, "plugin manifest mismatch")
				}
			}
		}

		return handler(srv, stream)
	}
}

func PatchWith(
	ctx context.Context,
	config v1beta1.PluginsSpec,
	receivedManifests *v1.ManifestList,
	lg *zap.SugaredLogger,
	gatewaySyncClient v1.PluginManifestClient,
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
			info := v.GetDataAndInfo()
			fullPluginPath := info.GetOpPath()
			if fullPluginPath == "" {
				fullPluginPath = filepath.Join(config.Dirs[0], pluginBaseName)
			}
			lg := lg.With("plugin", fullPluginPath)

			receivedData := info.GetData()

			switch info.GetOp() {
			case v1.PatchOp_CREATE:
				lg.Info("writing new plugin")
				err := os.WriteFile(fullPluginPath, receivedData, 0755)
				if err != nil {
					return err
				}
			case v1.PatchOp_UPDATE:
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
							_, err = gatewaySyncClient.UploadPatch(ctx, &v1.PatchSpec{
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
			case v1.PatchOp_REMOVE:
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
