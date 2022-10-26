package patch

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hashicorp/go-plugin"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
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

type inflightRequest struct {
	T *time.Timer
	C chan error
}

type FilesystemPluginSyncServer struct {
	controlv1.UnsafePluginManifestServer
	Logger *zap.SugaredLogger
	Config v1beta1.PluginsSpec

	loadMetadataOnce sync.Once
	pluginMetadata   *controlv1.ManifestMetadataList
	patchCache       PatchCache

	inflightUploadRequestsMu sync.Mutex
	inflightUploadRequests   map[string]inflightRequest
}

func NewFilesystemPluginSyncServer(cfg v1beta1.PluginsSpec, lg *zap.SugaredLogger) *FilesystemPluginSyncServer {
	return &FilesystemPluginSyncServer{
		Config:                 cfg,
		Logger:                 lg,
		inflightUploadRequests: make(map[string]inflightRequest),
		patchCache:             NewInMemoryCache(),
	}
}

func (f *FilesystemPluginSyncServer) getPluginMetadata() *controlv1.ManifestMetadataList {
	f.loadMetadataOnce.Do(func() {
		md, err := GetFilesystemPlugins(f.Config, f.Logger)
		if err != nil {
			panic(err)
		}
		f.pluginMetadata = md
	})
	return f.pluginMetadata
}

func GetFilesystemPlugins(config v1beta1.PluginsSpec, lg *zap.SugaredLogger) (*controlv1.ManifestMetadataList, error) {
	lg.Debug("plugin manifests requested")

	var matches []string
	for _, dir := range config.Dirs {
		items, err := plugin.Discover(plugins.DefaultPluginGlob, dir)
		if err == nil {
			matches = append(matches, items...)
		}
	}
	res := &controlv1.ManifestMetadataList{
		Items: map[string]*controlv1.ManifestMetadata{},
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
		res.Items[pluginPath] = &controlv1.ManifestMetadata{
			Hash: sum,
			Path: pluginPath,
			//Revision: *revision,
		}
	}
	return res, nil
}

func (f *FilesystemPluginSyncServer) SendManifestsOrKnownPatch(
	ctx context.Context,
	theirManifestMetadata *controlv1.ManifestMetadataList,
) (*controlv1.ManifestList, error) {
	// on startup
	ourManifestMetadata := f.getPluginMetadata()

	ops, err := ourManifestMetadata.LeftJoinOn(theirManifestMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile which manifests the gateway should send : %s", err)
	}
	f.Logger.Debug("%v manifests to send", ops)

	res := &controlv1.ManifestList{
		Manifests: make(map[string]*controlv1.CompressedManifest),
	}
	for pluginName, op := range ops.Items {
	RETRY_CACHE:
		if data, err := f.patchCache.Get(pluginName, op.OldHash, op.NewHash); err == nil {
			res.Manifests[pluginName] = &controlv1.CompressedManifest{
				DataAndInfo: &controlv1.ManifestData{
					Data:    data,
					OpPath:  op.TheirPath,
					Op:      op.Op,
					IsPatch: true,
					OldHash: op.OldHash,
					NewHash: op.NewHash,
				},
			}
		} else if op.Op == controlv1.PatchOp_REMOVE {
			res.Manifests[pluginName] = &controlv1.CompressedManifest{
				DataAndInfo: &controlv1.ManifestData{
					Data:    []byte{},
					OpPath:  op.TheirPath,
					Op:      op.Op,
					IsPatch: false,
					OldHash: op.OldHash,
					NewHash: op.NewHash,
				},
			}
		} else { // no known patch AND the agent plugin is not being removed
			data, err := os.ReadFile(op.OurPath)
			if err != nil {
				msgErr := fmt.Errorf("could not read a requested plugin in path '%s'(agentPath='%s', op='%s') : %s", op.OurPath, op.TheirPath, op.Op.String(), err)
				f.Logger.Error(msgErr)
				return nil, msgErr
			}
			// if there are no pending requests for an agent to upload this patch, ask this agent to upload it
			key := f.patchCache.Key(pluginName, op.OldHash, op.NewHash)
			lg := f.Logger.With(
				"plugin", pluginName,
			)

			f.inflightUploadRequestsMu.Lock()

			if ireq, ok := f.inflightUploadRequests[key]; ok {
				f.inflightUploadRequestsMu.Unlock()
				lg.Info("waiting on another agent to upload a patch for this plugin")
				select {
				case err := <-ireq.C:
					if err != nil {
						lg.With(
							zap.Error(err),
						).Warn("another agent failed to upload a patch for this plugin, retrying cache lookup")
					} else {
						lg.Info("patch was uploaded, retrying cache lookup")
					}
					goto RETRY_CACHE
				case <-ctx.Done():
					err := status.Error(codes.Canceled, "context canceled while waiting for patch to become available")
					return nil, err
				}
			}
			lg.Info("requesting agent to compute and upload patch for this plugin")
			// time this out after 60 seconds
			c := make(chan error, 1)
			f.inflightUploadRequests[key] = inflightRequest{
				T: time.AfterFunc(1*time.Minute, func() {
					f.inflightUploadRequestsMu.Lock()
					defer f.inflightUploadRequestsMu.Unlock()
					f.Logger.With(
						"plugin", pluginName,
					).Warn("timed out waiting for agent to upload patch")
					delete(f.inflightUploadRequests, key)
					c <- status.Error(codes.DeadlineExceeded, "timed out waiting for agent to upload patch")
					close(c)
				}),
				C: c,
			}

			f.inflightUploadRequestsMu.Unlock()

			res.Manifests[pluginName] = &controlv1.CompressedManifest{
				DataAndInfo: &controlv1.ManifestData{
					Data:               data,
					OpPath:             op.TheirPath,
					Op:                 op.Op,
					IsPatch:            false,
					RequestPatchUpload: true,
					OldHash:            op.OldHash,
					NewHash:            op.NewHash,
				},
			}
		}
	}
	return res, nil
}

func (f *FilesystemPluginSyncServer) GetPluginManifests(ctx context.Context, _ *emptypb.Empty) (*controlv1.ManifestMetadataList, error) {
	lg := f.Logger.With("method", "GetPluginManifests")

	return GetFilesystemPlugins(f.Config, lg)
}

func (f *FilesystemPluginSyncServer) UploadPatch(ctx context.Context, spec *controlv1.PatchSpec) (*emptypb.Empty, error) {
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
		if t.T.Stop() {
			// the timeout function was not called
			delete(f.inflightUploadRequests, key)
			t.C <- nil
			close(t.C)
		}
	}
	return &emptypb.Empty{}, nil
}

func (f *FilesystemPluginSyncServer) StreamServerInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		id := cluster.StreamAuthorizedID(stream.Context())

		//for now, plugin manifest validation is voluntary, but this may change in the future
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

// digests must match after the patchops are processed, not after the patch is calculated
func PatchWith(
	ctx context.Context,
	config v1beta1.PluginsSpec,
	receivedManifests *controlv1.ManifestList,
	lg *zap.SugaredLogger,
	gatewaySyncClient controlv1.PluginManifestClient,
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
			case controlv1.PatchOp_CREATE:
				lg.Infof("writing new plugin")
				err := os.WriteFile(fullPluginPath, receivedData, 0755)
				if err != nil {
					return err
				}
			case controlv1.PatchOp_UPDATE:
				if info.GetIsPatch() { // patch received from gateway
					lg.Infof("patching plugin with known patch revision: %s -> ", info.GetOldHash(), info.GetNewHash())
					outdatedRawBytes, err := os.ReadFile(fullPluginPath)
					if err != nil {
						return fmt.Errorf("could not find plugin binary : '%s' for plugin with name '%s': %s", fullPluginPath, pluginBaseName, err)
					}
					outdatedBufferedReader := bufio.NewReader(bytes.NewReader(outdatedRawBytes))
					newBufferedReader := bufio.NewReader(bytes.NewReader(receivedData))

					var patchResult bytes.Buffer
					err = ApplyIOStreamPatch(outdatedBufferedReader, newBufferedReader, &patchResult)
					if err != nil {
						return err
					}
					err = os.WriteFile(fullPluginPath, patchResult.Bytes(), 0755)
					if err != nil {
						return err
					}
				} else { // whole file received from gateway
					lg.Info("updating plugin without known patch")
					if info.RequestPatchUpload {
						outdatedPluginBytes, err := os.ReadFile(fullPluginPath)
						if err != nil {
							return fmt.Errorf("could not open the plugin '%s' : %s", fullPluginPath, err)
						}
						err = os.WriteFile(fullPluginPath, receivedData, 0755)
						if err != nil {
							return fmt.Errorf("could not write to the the plugin '%s' : %s", fullPluginPath, err)
						}
						outdatedBufferedReader := bufio.NewReader(bytes.NewBuffer(outdatedPluginBytes))
						newBufferedReader := bufio.NewReader(bytes.NewBuffer(receivedData))
						lg.Info("gateway requested patch upload for plugin : %s", fullPluginPath)
						oldHash := v.GetDataAndInfo().GetOldHash()
						newHash := v.GetDataAndInfo().GetNewHash()
						backgroundTasks.Add(1)
						go func() {
							defer func() {
								backgroundTasks.Done()
							}()
							lg.Info("computing patch")
							var patch bytes.Buffer
							err := GenerateIOStreamPatch(outdatedBufferedReader, newBufferedReader, &patch)
							if err != nil {
								lg.With(
									zap.Error(err),
								).Error("failed to generate patch")
								return
							}
							lg.Info("uploading computed patch")
							_, err = gatewaySyncClient.UploadPatch(ctx, &controlv1.PatchSpec{
								PluginName: pluginBaseName,
								OldHash:    oldHash,
								NewHash:    newHash,
								Patch:      patch.Bytes(),
							})
							if err != nil {
								lg.With(
									zap.Error(err),
								).Error("failed to upload patch")
							}
						}()
					} else { // go ahead and annihilate the old file
						err := os.WriteFile(fullPluginPath, receivedData, 0755)
						if err != nil {
							return err
						}
					}
				}
			case controlv1.PatchOp_REMOVE:
				lg.Infof("removing outdated plugin")
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
	}()

	return done, nil
}
