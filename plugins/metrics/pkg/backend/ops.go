package backend

import (
	"context"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type OpsServiceBackend struct {
	cortexops.UnsafeCortexOpsServer
	*MetricsBackend
}

var _ cortexops.CortexOpsServer = (*OpsServiceBackend)(nil)

func (m *OpsServiceBackend) GetDefaultConfiguration(ctx context.Context, in *driverutil.GetRequest) (*cortexops.CapabilityBackendConfigSpec, error) {
	m.WaitForInit()

	return m.ClusterDriver.GetDefaultConfiguration(ctx, in)
}

func (m *OpsServiceBackend) SetDefaultConfiguration(ctx context.Context, in *cortexops.CapabilityBackendConfigSpec) (*emptypb.Empty, error) {
	m.WaitForInit()

	return m.ClusterDriver.SetDefaultConfiguration(ctx, in)
}

func (m *OpsServiceBackend) ResetDefaultConfiguration(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	m.WaitForInit()

	return m.ClusterDriver.ResetDefaultConfiguration(ctx, in)
}

func (m *OpsServiceBackend) GetConfiguration(ctx context.Context, in *driverutil.GetRequest) (*cortexops.CapabilityBackendConfigSpec, error) {
	m.WaitForInit()

	return m.ClusterDriver.GetConfiguration(ctx, in)
}

func (m *OpsServiceBackend) SetConfiguration(ctx context.Context, in *cortexops.CapabilityBackendConfigSpec) (*emptypb.Empty, error) {
	m.WaitForInit()

	res, err := m.ClusterDriver.SetConfiguration(ctx, in)
	if err != nil {
		return nil, err
	}
	m.broadcastNodeSync(ctx)
	return res, nil
}

func (m *OpsServiceBackend) ResetConfiguration(ctx context.Context, in *cortexops.ResetRequest) (*emptypb.Empty, error) {
	m.WaitForInit()

	res, err := m.ClusterDriver.ResetConfiguration(ctx, in)
	if err != nil {
		return nil, err
	}
	m.broadcastNodeSync(ctx)
	return res, nil
}

func (m *OpsServiceBackend) Status(ctx context.Context, in *emptypb.Empty) (*driverutil.InstallStatus, error) {
	m.WaitForInit()

	return m.ClusterDriver.Status(ctx, in)
}

func (m *OpsServiceBackend) Install(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	m.WaitForInit()

	res, err := m.ClusterDriver.Install(ctx, in)
	if err != nil {
		return nil, err
	}
	m.broadcastNodeSync(ctx)
	return res, nil
}

func (m *OpsServiceBackend) Uninstall(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
	m.WaitForInit()

	clusters, err := m.StorageBackend.ListClusters(ctx, &corev1.LabelSelector{}, corev1.MatchOptions_Default)
	if err != nil {
		return nil, err
	}
	clustersWithCapability := []string{}
	for _, c := range clusters.GetItems() {
		if capabilities.Has(c, capabilities.Cluster(wellknown.CapabilityMetrics)) {
			clustersWithCapability = append(clustersWithCapability, c.Id)
		}
	}
	if len(clustersWithCapability) > 0 {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("There are %d agents sending metrics to the Opni monitoring backend. Uninstall the capability on all agents before attempting to uninstall the Opni monitoring backend.", len(clustersWithCapability)))
	}
	defer m.broadcastNodeSync(ctx)
	return m.ClusterDriver.Uninstall(ctx, in)
}

func (m *OpsServiceBackend) ListPresets(context.Context, *emptypb.Empty) (*cortexops.PresetList, error) {
	m.WaitForInit()

	return m.ClusterDriver.ListPresets(context.Background(), &emptypb.Empty{})
}

func (m *OpsServiceBackend) DryRun(ctx context.Context, req *cortexops.DryRunRequest) (*cortexops.DryRunResponse, error) {
	m.WaitForInit()

	return m.ClusterDriver.DryRun(ctx, req)
}

func (m *OpsServiceBackend) ConfigurationHistory(ctx context.Context, req *driverutil.ConfigurationHistoryRequest) (*cortexops.ConfigurationHistoryResponse, error) {
	m.WaitForInit()

	return m.ClusterDriver.ConfigurationHistory(ctx, req)
}
