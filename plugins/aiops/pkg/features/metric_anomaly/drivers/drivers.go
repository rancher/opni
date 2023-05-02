package drivers

import (
	"github.com/rancher/opni/pkg/plugins/driverutil"
)

type DashboardDriver interface {
	CreateDashboard(name string, jsonData string) error
	DeleteDashboard(name string) error
}

var DashboardDrivers = driverutil.NewDriverCache[DashboardDriver]()
