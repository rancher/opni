package backend

import (
	"context"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (m *MetricsBackend) GetClusterConfiguration(ctx context.Context, in *emptypb.Empty) (*cortexops.ClusterConfiguration, error) {
	m.WaitForInit()

	return m.ClusterDriver.GetClusterConfiguration(ctx, in)
}

func (m *MetricsBackend) ConfigureCluster(ctx context.Context, in *cortexops.ClusterConfiguration) (*emptypb.Empty, error) {
	m.WaitForInit()

	defer m.requestNodeSync(ctx, &corev1.Reference{})
	return m.ClusterDriver.ConfigureCluster(ctx, in)
}

func (m *MetricsBackend) GetClusterStatus(ctx context.Context, in *emptypb.Empty) (*cortexops.InstallStatus, error) {
	m.WaitForInit()

	return m.ClusterDriver.GetClusterStatus(ctx, in)
}

func (m *MetricsBackend) UninstallCluster(ctx context.Context, in *emptypb.Empty) (*emptypb.Empty, error) {
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
	defer m.requestNodeSync(ctx, &corev1.Reference{})
	return m.ClusterDriver.UninstallCluster(ctx, in)
}
