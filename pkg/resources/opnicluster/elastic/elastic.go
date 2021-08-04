package elastic

import (
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Reconciler struct {
	opniCluster *v1beta1.OpniCluster
	client      client.Client
}

func NewReconciler(client client.Client, opniCluster *v1beta1.OpniCluster) *Reconciler {
	return &Reconciler{
		client:      client,
		opniCluster: opniCluster,
	}
}

func (r *Reconciler) ElasticResources() (resourceList []resources.Resource, _ error) {
	resourceList = append(resourceList, r.elasticServices()...)
	resourceList = append(resourceList, r.elasticConfigSecret())
	resourceList = append(resourceList, r.elasticWorkloads()...)
	return
}
