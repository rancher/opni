package test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/plugins/logging/apis/loggingadmin"
	backenddriver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/backend"
	managementdriver "github.com/rancher/opni/plugins/logging/pkg/gateway/drivers/management"
	"github.com/rancher/opni/plugins/logging/pkg/util"
	loggingutil "github.com/rancher/opni/plugins/logging/pkg/util"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type MockManagementDriver struct {
	status           *util.MockInstallState
	clusterDetails   *loggingadmin.OpensearchClusterV2
	snapshots        map[string]*loggingadmin.SnapshotSchedule
	mSnapshots       sync.Mutex
	upgradeAvailable bool
}

func NewMockManagementDriver(stateTracker *util.MockInstallState) *MockManagementDriver {
	return &MockManagementDriver{
		status:           stateTracker,
		upgradeAvailable: true,
		snapshots:        map[string]*loggingadmin.SnapshotSchedule{},
	}
}

func (d *MockManagementDriver) AdminPassword(_ context.Context) ([]byte, error) {
	return []byte("testpassword"), nil
}

func (d *MockManagementDriver) NewOpensearchClientForCluster(context.Context) *opensearch.Client {
	transport := util.OpensearchMockTransport()

	client, err := opensearch.NewClient(
		opensearch.ClientConfig{
			URLs: []string{
				fmt.Sprintf(util.OpensearchURL),
			},
			Username:   "test",
			CertReader: util.GetMockCertReader(),
		},
		opensearch.WithTransport(transport),
	)
	if err != nil {
		panic(err)
	}

	return client
}

func (d *MockManagementDriver) GetCluster(_ context.Context) (*loggingadmin.OpensearchClusterV2, error) {
	if d.clusterDetails == nil {
		return &loggingadmin.OpensearchClusterV2{}, nil
	}

	return d.clusterDetails, nil
}

func (d *MockManagementDriver) DeleteCluster(_ context.Context) error {
	d.clusterDetails = nil
	d.status.Uninstall()
	return nil
}

func (d *MockManagementDriver) CreateOrUpdateCluster(
	_ context.Context,
	cluster *loggingadmin.OpensearchClusterV2,
	_ string,
	_ string,
) error {
	d.status.StartInstall()
	d.clusterDetails = cluster
	d.status.CompleteInstall()
	return nil
}

func (d *MockManagementDriver) UpgradeAvailable(_ context.Context, _ string) (bool, error) {
	return d.upgradeAvailable, nil
}

func (d *MockManagementDriver) DoUpgrade(_ context.Context, _ string) error {
	d.upgradeAvailable = false
	return nil
}

func (d *MockManagementDriver) GetStorageClasses(context.Context) ([]string, error) {
	return []string{
		"testclass",
	}, nil
}

func (d *MockManagementDriver) CreateOrUpdateSnapshotSchedule(_ context.Context, snapshot *loggingadmin.SnapshotSchedule, _ []string) error {
	d.mSnapshots.Lock()
	defer d.mSnapshots.Unlock()
	d.snapshots[snapshot.GetRef().GetName()] = snapshot
	return nil
}

func (d *MockManagementDriver) GetSnapshotSchedule(
	_ context.Context,
	ref *loggingadmin.SnapshotReference,
	_ []string,
) (*loggingadmin.SnapshotSchedule, error) {
	d.mSnapshots.Lock()
	defer d.mSnapshots.Unlock()

	s, ok := d.snapshots[ref.GetName()]
	if !ok {
		return nil, errors.New("snapshot not found")
	}

	return s, nil
}

func (d *MockManagementDriver) DeleteSnapshotSchedule(_ context.Context, ref *loggingadmin.SnapshotReference) error {
	d.mSnapshots.Lock()
	defer d.mSnapshots.Unlock()

	delete(d.snapshots, ref.GetName())
	return nil
}

func (d *MockManagementDriver) ListAllSnapshotSchedules(_ context.Context) (*loggingadmin.SnapshotStatusList, error) {
	statuses := []*loggingadmin.SnapshotStatus{}
	d.mSnapshots.Lock()
	defer d.mSnapshots.Unlock()
	for _, s := range d.snapshots {
		statuses = append(statuses, &loggingadmin.SnapshotStatus{
			Ref:    s.GetRef(),
			Status: "OK",
		})
	}
	return &loggingadmin.SnapshotStatusList{
		Statuses: statuses,
	}, nil
}

type clusterStatus struct {
	friendlyName string
	enabled      bool
	lastSyncTime time.Time
}

type MockBackendDriver struct {
	status   *util.MockInstallState
	clusters map[string]clusterStatus
	syncTime time.Time
	syncM    sync.RWMutex
}

func NewMockBackendDriver(stateTracker *util.MockInstallState) *MockBackendDriver {
	return &MockBackendDriver{
		status:   stateTracker,
		clusters: map[string]clusterStatus{},
	}
}

func (d *MockBackendDriver) Name() string {
	return "mock-driver"
}

func (d *MockBackendDriver) GetInstallStatus(_ context.Context) backenddriver.InstallState {
	switch {
	case d.status.IsCompleted():
		return backenddriver.Installed
	case d.status.IsStarted():
		return backenddriver.Pending
	default:
		return backenddriver.Absent
	}
}

func (d *MockBackendDriver) StoreCluster(_ context.Context, req *corev1.Reference, friendlyName string) error {
	d.clusters[req.GetId()] = clusterStatus{
		enabled:      true,
		friendlyName: friendlyName,
	}
	return nil
}

func (d *MockBackendDriver) StoreClusterMetadata(_ context.Context, id, name string) error {
	cluster, ok := d.clusters[id]
	if !ok {
		return fmt.Errorf("cluster not found")
	}

	cluster.friendlyName = name
	d.clusters[id] = cluster
	return nil
}

func (d *MockBackendDriver) DeleteCluster(_ context.Context, id string) error {
	delete(d.clusters, id)
	return nil
}

func (d *MockBackendDriver) SetClusterStatus(_ context.Context, id string, enabled bool) error {
	d.clusters[id] = clusterStatus{
		enabled:      enabled,
		lastSyncTime: time.Now(),
	}
	return nil
}

func (d *MockBackendDriver) GetClusterStatus(_ context.Context, id string) (*capabilityv1.NodeCapabilityStatus, error) {
	cluster, ok := d.clusters[id]
	if !ok {
		d.syncM.RLock()
		defer d.syncM.RUnlock()
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

func (d *MockBackendDriver) StoreClusterReadUser(_ context.Context, _, _, _ string) error {
	return nil
}

func (d *MockBackendDriver) SetSyncTime() {
	d.syncM.Lock()
	defer d.syncM.Unlock()
	d.syncTime = time.Now()
}

func init() {
	stateStore := &loggingutil.MockInstallState{}
	backenddriver.Drivers.Register("mock-driver", func(_ context.Context, _ ...driverutil.Option) (backenddriver.ClusterDriver, error) {
		return NewMockBackendDriver(stateStore), nil
	})
	managementdriver.Drivers.Register("mock-driver", func(_ context.Context, _ ...driverutil.Option) (managementdriver.ClusterDriver, error) {
		return NewMockManagementDriver(stateStore), nil
	})
}
