package drivers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	capabilityv1 "github.com/rancher/opni/pkg/apis/capability/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/samber/lo"
)

type InstallState int

const (
	Pending InstallState = iota
	Installed
	Absent
	Error
)

type ClusterDriver interface {
	Name() string
	CreateCredentials(context.Context, *corev1.Reference) error
	GetCredentials(context.Context, string) (username string, password string)
	GetExternalURL(context.Context) string
	GetInstallStatus(context.Context) InstallState
	SetClusterStatus(context.Context, string, bool) error
	GetClusterStatus(context.Context, string) (*capabilityv1.NodeCapabilityStatus, error)
	SetSyncTime()
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

func ListClusterDrivers() []string {
	lock.Lock()
	defer lock.Unlock()

	return lo.Keys(clusterDrivers)
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
