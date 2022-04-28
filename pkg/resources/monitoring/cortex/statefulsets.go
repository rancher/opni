package cortex

import "github.com/rancher/opni/pkg/resources"

func (r *Reconciler) statefulSets() []resources.Resource {
	compactor := r.buildCortexStatefulSet("compactor")
	storeGateway := r.buildCortexStatefulSet("store-gateway",
		ServiceName("cortex-store-gateway-headless"),
	)
	return []resources.Resource{
		compactor,
		storeGateway,
	}
}
