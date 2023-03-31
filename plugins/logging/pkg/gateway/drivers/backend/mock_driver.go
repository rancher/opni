package backend

import (
	"context"
	"time"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/logging/pkg/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type clusterStatus struct {
	enabled      bool
	lastSyncTime time.Time
}

type MockDriver struct {
	status   *util.MockInstallState
	clusters map[string]clusterStatus
	syncTime time.Time
}

func NewMockDriver(stateTracker *util.MockInstallState) *MockDriver {
	return &MockDriver{
		status:   stateTracker,
		clusters: map[string]clusterStatus{},
	}
}

func (d *MockDriver) Name() string {
	return "mock-driver"
}

func (d *MockDriver) GetInstallStatus(_ context.Context) InstallState {
	switch {
	case d.status.IsCompleted():
		return Installed
	case d.status.IsStarted():
		return Pending
	default:
		return Absent
	}
}

func (d *MockDriver) StoreCluster(_ context.Context, req *corev1.Reference) error {
	d.clusters[req.GetId()] = clusterStatus{
		enabled: true,
	}
	return nil
}

func (d *MockDriver) DeleteCluster(_ context.Context, id string) error {
	delete(d.clusters, id)
	return nil
}

func (d *MockDriver) SetClusterStatus(_ context.Context, id string, enabled bool) error {
	d.clusters[id] = clusterStatus{
		enabled:      enabled,
		lastSyncTime: time.Now(),
	}
	return nil
}

func (d *MockDriver) GetClusterStatus(_ context.Context, id string) (*capabilityv1.NodeCapabilityStatus, error) {
	cluster, ok := d.clusters[id]
	if !ok {
		return &capabilityv1.NodeCapabilityStatus{
			Enabled:  false,
			LastSync: timestamppb.New(d.syncTime),
		}, nil
	}

	return &capabilityv1.NodeCapabilityStatus{
		Enabled:  cluster.enabled,
		LastSync: timestamppb.New(cluster.lastSyncTime),
	}, nil
}

func (d *MockDriver) SetSyncTime() {
	d.syncTime = time.Now()
}
