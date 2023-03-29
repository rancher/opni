package backend

import (
	"context"
	"time"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	opnicorev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/storage"
	driver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
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
	case driver.Absent:
		return status.Error(codes.Unavailable, "opensearch cluster is not installed")
	case driver.Pending, driver.Installed:
		return nil
	case driver.Error:
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
		}
		warningErr = err
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

	addCtx, _ := context.WithTimeout(ctx, time.Minute*5)
	if err := b.waitForOpensearchClient(addCtx); err != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Error,
			Message: err.Error(),
		}, nil
	}

	cluster, err := b.MgmtClient.GetCluster(addCtx, req.Cluster)
	if err != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Error,
			Message: err.Error(),
		}, nil
	}

	if err := b.OpensearchManager.AddClusterMetadata(addCtx, cluster); err != nil {
		return &capabilityv1.InstallResponse{
			Status:  capabilityv1.InstallResponseStatus_Error,
			Message: err.Error(),
		}, nil
	}

	return &capabilityv1.InstallResponse{
		Status: capabilityv1.InstallResponseStatus_Success,
	}, nil
}

func (b *LoggingBackend) InstallerTemplate(context.Context, *emptypb.Empty) (*capabilityv1.InstallerTemplateResponse, error) {
	return &capabilityv1.InstallerTemplateResponse{
		Template: `helm install opni-agent ` +
			`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
			`oci://docker.io/rancher/opni-agent --version=0.5.4 ` +
			`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
			`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
			`--create-namespace`,
	}, nil
}
