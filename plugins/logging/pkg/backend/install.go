package backend

import (
	"context"
	"fmt"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/plugins/logging/pkg/gateway/drivers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (b *LoggingBackend) CanInstall(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	b.WaitForInit()
	err := b.canInstall(ctx)

	return &emptypb.Empty{}, err
}

func (b *LoggingBackend) canInstall(ctx context.Context) error {
	installState := b.ClusterDriver.GetInstallStatus(ctx)
	switch installState {
	case drivers.Absent:
		return status.Error(codes.Unavailable, "opensearch cluster is not installed")
	case drivers.Pending, drivers.Installed:
		return nil
	case drivers.Error:
		fallthrough
	default:
		return status.Error(codes.Internal, "unknown opensearch cluster state")
	}
}

func (b *LoggingBackend) Install(ctx context.Context, req *capabilityv1.InstallRequest) (*capabilityv1.InstallResponse, error) {
	b.WaitForInit()

	var warningErr error
	if err := b.canInstall(ctx); err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		} else {
			warningErr = err
		}
	}

	if err := b.ClusterDriver.CreateCredentials(ctx, req.GetCluster()); err != nil {
		if !req.IgnoreWarnings {
			return &capabilityv1.InstallResponse{
				Status:  capabilityv1.InstallResponseStatus_Error,
				Message: err.Error(),
			}, nil
		} else {
			warningErr = err
		}
	}

	_, err := b.StorageBackend.UpdateCluster(ctx, req.Cluster,
		storage.NewAddCapabilityMutator[*opnicorev1.Cluster](capabilities.Cluster(wellknown.CapabilityLogs)),
	)
	if err != nil {
		return nil, err
	}

	b.requestNodeSync(ctx, req.Cluster)

	if warningErr != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Warning,
			Message: warningErr.Error(),
		}, nil
	}
	return &capabilityv1.InstallResponse{
		Status: capabilityv1.InstallResponseStatus_Success,
	}, nil
}

func (b *LoggingBackend) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return &capabilityv1.InstallerTemplateResponse{
		Template: fmt.Sprintf(`opni bootstrap logging {{ arg "input" "Opensearch Cluster Name" "+required" "+default:%s" }} `, b.OpensearchCluster.Name) +
			`{{ arg "select" "Kubernetes Provider" "" "rke" "rke2" "k3s" "aks" "eks" "gke" "+omitEmpty" "+format:--provider={{ value }}" }} ` +
			`--token={{ .Token }} --pin={{ .Pin }} ` +
			`--gateway-url={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }}`,
	}, nil
}
