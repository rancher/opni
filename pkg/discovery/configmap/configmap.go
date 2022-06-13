/// CRUD user defined ConfigMap for discovering their custom services
package configmap

import (
	"context"

	discovery "github.com/rancher/opni/pkg/discovery/v1alpha"
)

type ConfigMapService struct {
	service discovery.Service
}

func (c *ConfigMapService) Discover(ctx context.Context) error {
	return c.service.Discover(ctx)
}
