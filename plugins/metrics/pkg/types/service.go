package types

import (
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util"
)

type Service interface {
	Activate() error
}

// An optional interface for services which implement a plugin interface
// and need to add themselves to the scheme. Some services (e.g. api extensions)
// are aggregated by the plugin host and will not need to implement this.
type PluginService interface {
	AddToScheme(meta.Scheme)
}

type ManagementService interface {
	ManagementServices() []util.ServicePackInterface
}

type StreamService interface {
	StreamServices() []util.ServicePackInterface
}

var (
	Services = driverutil.NewCache[Service]()
)
