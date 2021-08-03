package elastic

import (
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
)

type Reconciler struct {
	opniCluster *v1beta1.OpniCluster
}

func NewReconciler(opniCluster *v1beta1.OpniCluster) *Reconciler {
	return &Reconciler{
		opniCluster: opniCluster,
	}
}

func (r *Reconciler) ElasticResources() (resourceList []resources.Resource, _ error) {
	resourceList = append(resourceList, r.elasticServices()...)
	resourceList = append(resourceList, r.elasticConfigSecret())
	resourceList = append(resourceList, r.elasticWorkloads()...)
	return
}
