package elastic

import (
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

type Reconciler struct {
	opniCluster *v1beta1.OpniCluster
}

func (r *Reconciler) elastic() (resourceList []resources.Resource, retErr error) {
	services := r.elasticServices()
	configSecret := r.elasticConfigSecret()

	return append(services, configSecret), nil
}
