package patch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ controlv1.PluginSyncServer = (*FilesystemPluginSyncServer)(nil)

type FilesystemPluginSyncServer struct {
	controlv1.UnsafePluginSyncServer
	logger           *zap.SugaredLogger
	config           v1beta1.PluginsSpec
	loadMetadataOnce sync.Once
	manifest         *controlv1.PluginManifest
	patchCache       Cache
}

func NewFilesystemPluginSyncServer(cfg v1beta1.PluginsSpec, lg *zap.SugaredLogger) (*FilesystemPluginSyncServer, error) {
	var patchEngine BinaryPatcher
	switch cfg.Cache.PatchEngine {
	case v1beta1.PatchEngineBsdiff:
		patchEngine = BsdiffPatcher{}
	default:
		return nil, fmt.Errorf("unknown patch engine: %s", cfg.Cache.PatchEngine)
	}

	var pluginCache Cache
	switch cfg.Cache.Backend {
	case v1beta1.CacheBackendFilesystem:
		var err error
		pluginCache, err = NewFilesystemCache(cfg.Cache.Filesystem, patchEngine, lg.Named("cache"))
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown cache backend: %s", cfg.Cache.Backend)
	}

	return &FilesystemPluginSyncServer{
		config:     cfg,
		logger:     lg,
		patchCache: pluginCache,
	}, nil
}

func (f *FilesystemPluginSyncServer) RunGarbageCollection(ctx context.Context, store storage.ClusterStore) error {
	clusters, err := store.ListClusters(ctx, &corev1.LabelSelector{}, 0)
	if err != nil {
		return err
	}
	digestsToKeep := f.getPluginManifest().DigestSet()
	for _, cluster := range clusters.Items {
		versions := cluster.GetMetadata().GetLastKnownConnectionDetails().GetPluginVersions()
		for _, v := range versions {
			digestsToKeep[v] = struct{}{}
		}
	}
	curDigests, err := f.patchCache.ListDigests()
	if err != nil {
		return err
	}
	var toClean []string
	for _, h := range curDigests {
		if _, ok := digestsToKeep[h]; !ok {
			toClean = append(toClean, h)
		}
	}
	f.logger.Info("running plugin cache gc")
	f.patchCache.Clean(toClean...)
	return nil
}

func (f *FilesystemPluginSyncServer) getPluginManifest() *controlv1.PluginManifest {
	f.loadMetadataOnce.Do(f.loadPluginManifest)
	return f.manifest
}

func (f *FilesystemPluginSyncServer) loadPluginManifest() {
	if f.manifest != nil {
		panic("bug: tried to call loadPluginManifest twice")
	}
	md, err := GetFilesystemPlugins(f.config, f.logger)
	if err != nil {
		panic(err)
	}
	if err := f.patchCache.Archive(md); err != nil {
		panic(fmt.Sprintf("failed to archive plugin manifest: %v", err))
	}
	f.manifest = md.ToManifest()
}

func (f *FilesystemPluginSyncServer) SyncPluginManifest(
	ctx context.Context,
	theirManifest *controlv1.PluginManifest,
) (*controlv1.SyncResults, error) {
	// on startup
	ourManifest := f.getPluginManifest()
	archive := LeftJoinOn(ourManifest, theirManifest)

	errg, ctx := errgroup.WithContext(ctx)
	for _, entry := range archive.Items {
		entry := entry
		errg.Go(func() error {
			switch entry.Op {
			case controlv1.PatchOp_Create:
				data, err := f.patchCache.GetPlugin(entry.NewDigest)
				if err != nil {
					f.logger.With(
						zap.Error(err),
						"plugin", entry.Module,
						"filename", entry.Filename,
					).Errorf("lost plugin in cache")
					return status.Errorf(codes.Internal, "lost plugin in cache: %s", entry.Module)
				}
				entry.Data = data
			case controlv1.PatchOp_Update:
				// fetch existing patch or wait for a patch to be calculated
				lg := f.logger.With(
					"plugin", entry.Module,
					"oldDigest", entry.OldDigest,
					"newDigest", entry.NewDigest,
				)
				if data, err := f.patchCache.RequestPatch(entry.OldDigest, entry.NewDigest); err == nil {
					// send known patch
					entry.Data = data
				} else if errors.Is(err, os.ErrNotExist) {
					// no patch can ever be calculated in this case
					data, err := f.patchCache.GetPlugin(entry.NewDigest)
					if err != nil {
						lg.With(
							zap.Error(err),
						).Errorf("lost plugin in cache")
						return status.Errorf(codes.Internal, "lost plugin in cache, cannot generate patch: %s", entry.Module)
					}
					entry.Data = data
					entry.Op = controlv1.PatchOp_Create
				} else {
					lg.With(
						zap.Error(err),
					).Errorf("error requesting patch for plugin %s %s->%s", entry.Module, entry.OldDigest, entry.NewDigest)
					return status.Errorf(codes.Internal, "internal error in plugin cache, cannot sync: %s", entry.Module)
				}
			}
			return nil
		})
	}
	if err := errg.Wait(); err != nil {
		return nil, err
	}
	return &controlv1.SyncResults{
		DesiredState:    ourManifest,
		RequiredPatches: archive,
	}, nil
}

func (f *FilesystemPluginSyncServer) GetPluginManifest(ctx context.Context, _ *emptypb.Empty) (*controlv1.PluginManifest, error) {
	return f.getPluginManifest(), nil
}

type manifestMetadataKeyType struct{}

var manifestMetadataKey = manifestMetadataKeyType{}

func ManifestMetadataFromContext(ctx context.Context) (*controlv1.PluginManifest, bool) {
	md, ok := ctx.Value(manifestMetadataKey).(*controlv1.PluginManifest)
	return md, ok
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
				if f.getPluginManifest().Digest() != digest {
					f.logger.With(
						"id", id,
					).Info("agent plugins are out of date; requesting update")
					return status.Errorf(codes.FailedPrecondition, "plugins are out of date")
				}
			}
		}

		return handler(srv, &util.ServerStreamWithContext{
			Stream: stream,
			Ctx:    context.WithValue(stream.Context(), manifestMetadataKey, f.getPluginManifest()),
		})
	}
}

func (f *FilesystemPluginSyncServer) Collectors() []prometheus.Collector {
	return f.patchCache.MetricsCollectors()
}
