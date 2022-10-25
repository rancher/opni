package drivers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rancher/opni/pkg/alerting/routing"

	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
)

type ClusterDriver interface {
	alertops.AlertingAdminServer
	alertops.DynamicAlertingServer
	// Unique name of the driver
	Name() string
	// ShouldDisableNode is called during node sync for nodes which otherwise
	// have this capability enabled. If this function returns an error, the
	// node will be set to disabled instead, and the error will be logged.
	ShouldDisableNode(*corev1.Reference) error

	// !! Read only view of alerting options
	GetRuntimeOptions() (shared.NewAlertingOptions, error)
	ConfigFromBackend(ctx context.Context) (
		*routing.RoutingTree,
		*routing.OpniInternalRouting,
		error,
	)
	ApplyConfigToBackend(
		ctx context.Context,
		config *routing.RoutingTree,
		internal *routing.OpniInternalRouting,
	) error
}

var (
	lock                 = &sync.Mutex{}
	clusterDrivers       map[string]ClusterDriver
	persistentDrivers    []func() ClusterDriver
	failedClusterDrivers map[string]string
)

func init() {
	ResetClusterDrivers()
}

func RegisterClusterDriver(driver ClusterDriver) {
	lock.Lock()
	defer lock.Unlock()

	if _, ok := clusterDrivers[driver.Name()]; ok {
		panic("driver already exists: " + driver.Name())
	}
	clusterDrivers[driver.Name()] = driver
}

func RegisterPersistentClusterDriver(driverFunc func() ClusterDriver) {
	lock.Lock()
	defer lock.Unlock()

	persistentDrivers = append(persistentDrivers, driverFunc)
}

func LogClusterDriverFailure(name string, err error) {
	lock.Lock()
	defer lock.Unlock()

	failedClusterDrivers[name] = err.Error()
}

func GetClusterDriver(name string) (ClusterDriver, error) {
	lock.Lock()
	defer lock.Unlock()

	driver, ok := clusterDrivers[name]
	if !ok {
		if failureMsg, ok := failedClusterDrivers[name]; ok {
			return nil, errors.New(failureMsg)
		}
		return nil, fmt.Errorf("driver not found")
	}
	return driver, nil
}

func ResetClusterDrivers() {
	lock.Lock()
	clusterDrivers = make(map[string]ClusterDriver)
	failedClusterDrivers = make(map[string]string)
	lock.Unlock()
	for _, driverFunc := range persistentDrivers {
		RegisterClusterDriver(driverFunc())
	}
}

type NoopClusterDriver struct {
	alertops.UnimplementedAlertingAdminServer
	alertops.DynamicAlertingServer
}

func (d *NoopClusterDriver) ConfigFromBackend(ctx context.Context) (*routing.RoutingTree, *routing.OpniInternalRouting, error) {
	return nil, nil, fmt.Errorf("no config from backend")
}

func (d *NoopClusterDriver) ApplyConfigToBackend(
	ctx context.Context,
	config *routing.RoutingTree,
	internal *routing.OpniInternalRouting,
) error {
	return fmt.Errorf("nothing to apply to backend")
}

func (d *NoopClusterDriver) Name() string {
	return "noop"
}

func (d *NoopClusterDriver) ShouldDisableNode(*corev1.Reference) error {
	// the noop driver will never forcefully disable a node
	return nil
}

func (d *NoopClusterDriver) GetRuntimeOptions() (shared.NewAlertingOptions, error) {
	return shared.NewAlertingOptions{}, fmt.Errorf("noop driver does not have alerting runtime options")
}

var _ ClusterDriver = &NoopClusterDriver{}
