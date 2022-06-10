/// Custom agent for monitoring pod creation/ deletion via
/// kube server

package kubernetes

import (
	"context"

	discovery "github.com/rancher/opni/pkg/discovery/v1alpha"
)

type KubernetesService struct {
	service discovery.Service
}

func (k *KubernetesService) Discover(ctx context.Context) error {
	return k.service.Discover(ctx)
}
