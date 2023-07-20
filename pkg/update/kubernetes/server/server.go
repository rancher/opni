package server

import (
	"context"
	"fmt"
	"os"

	"github.com/prometheus/client_golang/prometheus"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/oci"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/kubernetes"
	"github.com/rancher/opni/pkg/urn"
	opniurn "github.com/rancher/opni/pkg/urn"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type kubernetesSyncServer struct {
	imageFetcher oci.Fetcher
	lg           *zap.SugaredLogger
}

type kubernetesOptions struct {
	namespace string
}

func (o *kubernetesOptions) apply(opts ...KubernetesOption) {
	for _, opt := range opts {
		opt(o)
	}
}

type KubernetesOption func(*kubernetesOptions)

func WithNamespace(namespace string) KubernetesOption {
	return func(o *kubernetesOptions) {
		o.namespace = namespace
	}
}

func NewKubernetesSyncServer(
	conf v1beta1.KubernetesAgentUpgradeSpec,
	lg *zap.SugaredLogger,
	opts ...KubernetesOption,
) (update.UpdateTypeHandler, error) {
	options := kubernetesOptions{
		namespace: os.Getenv("POD_NAMESPACE"),
	}
	options.apply(opts...)

	imageFetcher, err := machinery.ConfigureOCIFetcher(string(conf.ImageResolver), options.namespace)
	if err != nil {
		return nil, err
	}

	return &kubernetesSyncServer{
		imageFetcher: imageFetcher,
		lg:           lg,
	}, nil
}

func (k *kubernetesSyncServer) Strategy() string {
	return kubernetes.UpdateStrategy
}

func (k *kubernetesSyncServer) Collectors() []prometheus.Collector {
	return []prometheus.Collector{}
}

func (k *kubernetesSyncServer) CalculateUpdate(
	ctx context.Context,
	manifest *controlv1.UpdateManifest,
) (*controlv1.PatchList, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	updateType, err := update.GetType(manifest.GetItems())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	switch updateType {
	case opniurn.Agent:
		return k.calculateAgentUpdate(ctx, manifest)
	default:
		return nil, status.Error(codes.Unimplemented, kubernetes.ErrUnhandledUpdateType(string(updateType)).Error())
	}
}

type manifestMetadataKeyType struct{}

var manifestMetadataKey = manifestMetadataKeyType{}

func (*kubernetesSyncServer) ManifestMetadataFromContext(ctx context.Context) (*controlv1.UpdateManifest, bool) {
	md, ok := ctx.Value(manifestMetadataKey).(*controlv1.UpdateManifest)
	return md, ok
}

func ManifestMetadataFromContext(ctx context.Context) (*controlv1.UpdateManifest, bool) {
	return (*kubernetesSyncServer)(nil).ManifestMetadataFromContext(ctx)
}

func (k *kubernetesSyncServer) CalculateExpectedManifest(ctx context.Context, updateType urn.UpdateType) (*controlv1.UpdateManifest, error) {
	if updateType != opniurn.Agent {
		return nil, status.Error(codes.Unimplemented, kubernetes.ErrUnhandledUpdateType(string(updateType)).Error())
	}
	expectedManifest := &controlv1.UpdateManifest{}
	strategy := metadata.ValueFromIncomingContext(ctx, controlv1.UpdateStrategyKey)
	if len(strategy) != 1 {
		return nil, status.Error(codes.InvalidArgument, "update strategy missing or invalid")
	}
	for component, updateType := range kubernetes.ComponentImageMap {
		image, err := k.imageFetcher.GetImage(ctx, updateType)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		newEntry := &controlv1.UpdateManifestEntry{
			Package: urn.NewOpniURN(opniurn.Agent, strategy[0], string(component)).String(),
			Path:    image.Path(),
			Digest:  image.Digest.String(),
		}
		expectedManifest.Items = append(expectedManifest.Items, newEntry)
	}
	return expectedManifest, nil
}

func (k *kubernetesSyncServer) calculateAgentUpdate(
	ctx context.Context,
	manifest *controlv1.UpdateManifest,
) (*controlv1.PatchList, error) {
	patches := []*controlv1.PatchSpec{}

	for _, item := range manifest.GetItems() {
		image, err := k.imageForEntry(ctx, item)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		if image == nil || image.Empty() {
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("no image found for %s", item.GetPackage()))
		}
		patch := patchForImage(item, image)
		patches = append(patches, patch)
	}
	patchList := &controlv1.PatchList{
		Items: patches,
	}

	return patchList, nil
}

func (k *kubernetesSyncServer) imageForEntry(
	ctx context.Context,
	entry *controlv1.UpdateManifestEntry,
) (*oci.Image, error) {
	urn, err := opniurn.ParseString(entry.GetPackage())
	if err != nil {
		return nil, err
	}

	component := kubernetes.ComponentType(urn.Component)

	imageType, ok := kubernetes.ComponentImageMap[component]
	if !ok {
		k.lg.Warnf("no image found for component %s", component)
		return nil, nil
	}

	return k.imageFetcher.GetImage(ctx, imageType)
}

func patchForImage(
	entry *controlv1.UpdateManifestEntry,
	image *oci.Image,
) *controlv1.PatchSpec {
	if entry == nil {
		return nil
	}
	existingImage, err := oci.Parse(entry.GetPath())
	if err != nil {
		return nil
	}

	if entry.GetDigest() == image.DigestOrTag() && existingImage.Repository == image.Repository {
		return &controlv1.PatchSpec{
			Package: entry.GetPackage(),
			Path:    entry.GetPath(),
			Op:      controlv1.PatchOp_None,
		}
	}
	return &controlv1.PatchSpec{
		Op:        controlv1.PatchOp_Update,
		OldDigest: entry.GetDigest(),
		NewDigest: image.DigestOrTag(),
		Package:   entry.GetPackage(),
		Path:      image.Path(),
	}
}
