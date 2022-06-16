/// Shared datatypes for downstream service discovery

/// There are several ways to do downstream service discovery:
/// 1. Prometheus comes with a built in auto-discovery:
/// 2. Have a custom agent that uses kubernetes built in to monitor pod creation and deletion
/// 3. Monitor/ User create ConfigMaps to define the downstream services

package discovery

import (
	"context"
)

/// Returns a map of ServiceId -> ServiceName
type DiscoveryRule func(ctx context.Context) ([]Service, error)

type Discoverable interface {
	Discover(ctx context.Context, drules ...DiscoveryRule) error
}

type ServiceGroup struct {
	Services []Discoverable
}

/// Service base class
type Service interface {
	GetClusterId() string
	GetServiceName() string
	GetServiceId() string
}
