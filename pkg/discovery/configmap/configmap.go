/// CRUD user defined ConfigMap for discovering their custom services
package configmap

import (
	discovery "github.com/rancher/opni/pkg/discovery"
)

type ConfigMapService struct {
	service discovery.Service
}
