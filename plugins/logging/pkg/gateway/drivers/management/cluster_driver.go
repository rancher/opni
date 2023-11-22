package management

import (
	"context"

	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
)

type ClusterDriver interface {
	AdminPassword(context.Context) ([]byte, error)
	NewOpensearchClientForCluster(context.Context) *opensearch.Client
	GetCluster(context.Context) (*loggingadmin.OpensearchClusterV2, error)
	DeleteCluster(context.Context) error
	CreateOrUpdateCluster(ctx context.Context, cluster *loggingadmin.OpensearchClusterV2, opniVersion string, natsName string) error
	UpgradeAvailable(ctx context.Context, opniVersion string) (bool, error)
	DoUpgrade(ctx context.Context, opniVersion string) error
	GetStorageClasses(context.Context) ([]string, error)
	CreateOrUpdateSnapshotSchedule(ctx context.Context, snapshot *loggingadmin.SnapshotSchedule, defaultIndices []string) error
	GetSnapshotSchedule(ctx context.Context, ref *loggingadmin.SnapshotReference, defaultIndices []string) (*loggingadmin.SnapshotSchedule, error)
	DeleteSnapshotSchedule(ctx context.Context, ref *loggingadmin.SnapshotReference) error
	ListAllSnapshotSchedules(ctx context.Context) (*loggingadmin.SnapshotStatusList, error)
}

var Drivers = driverutil.NewCache[ClusterDriver]()
