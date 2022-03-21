package elastic

import (
	"context"

	"github.com/opensearch-project/opensearch-go"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OpniMasterWorkload = "opni-es-master"
	OpniDataWorkload   = "opni-es-data"
	OpniClientWorkload = "opni-es-client"
	OpniKibanaWorkload = "opni-es-kibana"
)

type Reconciler struct {
	opniCluster *v1beta2.OpniCluster
	client      client.Client
	ctx         context.Context
	esClient    *opensearch.Client
}

func NewReconciler(ctx context.Context, client client.Client, opniCluster *v1beta2.OpniCluster) *Reconciler {
	return &Reconciler{
		client:      client,
		opniCluster: opniCluster,
		ctx:         ctx,
	}
}

func (r *Reconciler) ElasticResources() (resourceList []resources.Resource, _ error) {
	// Generate the elasticsearch password resources and return any errors
	err := r.elasticPasswordResourcces()
	if err != nil {
		return resourceList, err
	}

	resourceList = append(resourceList, r.elasticServices()...)
	resourceList = append(resourceList, r.elasticConfigSecret())
	resourceList = append(resourceList, r.elasticWorkloads()...)
	return
}
