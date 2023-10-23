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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log/slog"
)

type kubernetesSyncServer struct {
	imageFetcher oci.Fetcher
	lg           *slog.Logger
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
	lg *slog.Logger,
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

func (k *kubernetesSyncServer) CalculateExpectedManifest(ctx context.Context, updateType urn.UpdateType) (*controlv1.UpdateManifest, error) {
	if updateType != opniurn.Agent {
		return nil, status.Error(codes.Unimplemented, kubernetes.ErrUnhandledUpdateType(string(updateType)).Error())
	}
	expectedManifest := &controlv1.UpdateManifest{}
	strategyKey := controlv1.UpdateStrategyKeyForType(updateType)
	strategy := metadata.ValueFromIncomingContext(ctx, strategyKey)
	if len(strategy) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "update strategy for type %q missing (key: %s)", updateType, strategyKey)
	} else if len(strategy) > 1 {
		return nil, status.Errorf(codes.InvalidArgument, "expected 1 value for metadata key %s, got %d", strategyKey, len(strategy))
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
	current *controlv1.UpdateManifest,
) (*controlv1.PatchList, error) {
	patches := []*controlv1.PatchSpec{}

	expected, err := k.CalculateExpectedManifest(ctx, opniurn.Agent)
	if err != nil {
		return nil, err
	}

	expectedItems := map[string]*controlv1.UpdateManifestEntry{}
	for _, entry := range expected.GetItems() {
		urn, err := opniurn.ParseString(entry.GetPackage())
		if err != nil {
			return nil, err
		}
		expectedItems[urn.Component] = entry
	}

	for _, item := range current.GetItems() {
		urn, err := opniurn.ParseString(item.GetPackage())
		if err != nil {
			return nil, err
		}
		expectedItem, ok := expectedItems[urn.Component]
		if !ok {
			// TODO: implement all patch operations
			return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("no image found for %s", item.GetPackage()))
		}

		patches = append(patches, patchForEntry(item, expectedItem))
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
		k.lg.Warn(fmt.Sprintf("no image found for component %s", component))
		return nil, nil
	}

	return k.imageFetcher.GetImage(ctx, imageType)
}

func patchForEntry(
	oldEntry, newEntry *controlv1.UpdateManifestEntry,
) *controlv1.PatchSpec {
	if oldEntry == nil {
		return nil
	}

	existingImage, err := oci.Parse(oldEntry.GetPath())
	if err != nil {
		return nil
	}
	newImage, err := oci.Parse(newEntry.GetPath())
	if err != nil {
		return nil
	}

	if oldEntry.GetDigest() == newEntry.GetDigest() && existingImage.Repository == newImage.Repository {

		return &controlv1.PatchSpec{
			Package: oldEntry.GetPackage(),
			Path:    oldEntry.GetPath(),
			Op:      controlv1.PatchOp_None,
		}
	}
	return &controlv1.PatchSpec{
		Op:        controlv1.PatchOp_Update,
		OldDigest: oldEntry.GetDigest(),
		NewDigest: newEntry.GetDigest(),
		Package:   oldEntry.GetPackage(),
		Path:      newEntry.GetPath(),
	}
}
