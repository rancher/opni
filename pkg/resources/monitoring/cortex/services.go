package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) services() []resources.Resource {
	resources := []resources.Resource{
		r.memberlistService(),
	}
	resources = append(resources, r.buildCortexWorkloadServices("alertmanager",
		AddServiceMonitor(),
	)...)
	resources = append(resources, r.buildCortexWorkloadServices("compactor",
		AddServiceMonitor(),
	)...)
	resources = append(resources, r.buildCortexWorkloadServices("distributor",
		AddHeadlessService(true),
		AddServiceMonitor(),
	)...)
	resources = append(resources, r.buildCortexWorkloadServices("ingester",
		AddHeadlessService(false),
		AddServiceMonitor(),
	)...)
	resources = append(resources, r.buildCortexWorkloadServices("querier",
		AddServiceMonitor(),
	)...)
	resources = append(resources, r.buildCortexWorkloadServices("query-frontend",
		AddHeadlessService(true),
		AddServiceMonitor(),
	)...)
	resources = append(resources, r.buildCortexWorkloadServices("ruler",
		AddServiceMonitor(),
	)...)
	resources = append(resources, r.buildCortexWorkloadServices("store-gateway",
		AddHeadlessService(false),
		AddServiceMonitor(),
	)...)
	return resources
}

func (r *Reconciler) memberlistService() resources.Resource {
	memberlist := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex-memberlist",
			Namespace: r.mc.Namespace,
			Labels:    cortexAppLabel,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone,
			Ports: []corev1.ServicePort{
				{
					Name:       "gossip",
					Port:       7946,
					TargetPort: intstr.FromString("gossip"),
				},
			},
			Selector: map[string]string{
				"app.kubernetes.io/name":     "cortex",
				"app.kubernetes.io/part-of":  "opni",
				"app.kubernetes.io/instance": "cortex",
			},
		},
	}
	ctrl.SetControllerReference(r.mc, memberlist, r.client.Scheme())
	return resources.PresentIff(r.mc.Spec.Cortex.Enabled, memberlist)
}
