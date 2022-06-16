/// Custom agent for monitoring pod creation/ deletion via
/// kube server

package kubernetes

import (
	discovery "github.com/rancher/opni/pkg/discovery"
)

type KubernetesService struct {
	service discovery.Service
}
