package management

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/opensearch/opensearch"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/logging/pkg/apis/loggingadmin"
	"github.com/rancher/opni/plugins/logging/pkg/util"
	corev1 "k8s.io/api/core/v1"
)

type MockDriver struct {
	status           *util.MockInstallState
	clusterDetails   *loggingadmin.OpensearchClusterV2
	upgradeAvailable bool
}

func NewMockDriver(stateTracker *util.MockInstallState) *MockDriver {
	return &MockDriver{
		status:           stateTracker,
		upgradeAvailable: true,
	}
}

func (d *MockDriver) Name() string {
	return "mock-driver"
}

func (d *MockDriver) AdminPassword(_ context.Context) ([]byte, error) {
	return []byte("testpassword"), nil
}

func (d *MockDriver) NewOpensearchClientForCluster(context.Context) *opensearch.Client {
	transport := util.OpensearchMockTransport()
	certMgr := &test.TestCertManager{}

	client, err := opensearch.NewClient(
		opensearch.ClientConfig{
			URLs: []string{
				fmt.Sprintf(util.OpensearchURL),
			},
			Username:   "test",
			CertReader: certMgr,
		},
		opensearch.WithTransport(transport),
	)
	if err != nil {
		panic(err)
	}

	return client
}

func (d *MockDriver) GetCluster(_ context.Context) (*loggingadmin.OpensearchClusterV2, error) {
	if d.clusterDetails == nil {
		return &loggingadmin.OpensearchClusterV2{}, nil
	}

	return d.clusterDetails, nil
}

func (d *MockDriver) DeleteCluster(_ context.Context) error {
	d.clusterDetails = nil
	return nil
}

func (d *MockDriver) CreateOrUpdateCluster(
	_ context.Context,
	cluster *loggingadmin.OpensearchClusterV2,
	_ string,
	_ *corev1.LocalObjectReference,
) error {
	d.clusterDetails = cluster
	return nil
}

func (d *MockDriver) UpgradeAvailable(_ context.Context, _ string) (bool, error) {
	return d.upgradeAvailable, nil
}

func (d *MockDriver) DoUpgrade(_ context.Context, _ string) error {
	d.upgradeAvailable = false
	return nil
}

func (d *MockDriver) GetStorageClasses(context.Context) ([]string, error) {
	return []string{
		"testclass",
	}, nil
}
