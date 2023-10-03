package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) services() []resources.Resource {
	svcOpts := []CortexServiceOption{
		AddServiceMonitor(),
	}
	if targets := r.mc.Spec.Cortex.CortexWorkloads.GetTargets(); len(targets) == 1 &&
		lo.ValueOr(targets, all, nil) != nil {
		svcOpts = append(svcOpts, WithTargetLabelOverride(all))
	}

	resources := []resources.Resource{
		r.memberlistService(),
	}
	resources = append(resources, r.buildCortexWorkloadServices(
		alertmanager,
		svcOpts...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		compactor,
		svcOpts...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		purger,
		svcOpts...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		distributor,
		append(svcOpts, AddHeadlessService(true))...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		ingester,
		append(svcOpts, AddHeadlessService(false))...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		querier,
		svcOpts...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		queryFrontend,
		append(svcOpts, AddHeadlessService(true))...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		ruler,
		svcOpts...,
	)...)
	resources = append(resources, r.buildCortexWorkloadServices(
		storeGateway,
		append(svcOpts, AddHeadlessService(false))...,
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
	return resources.PresentIff(lo.FromPtr(r.mc.Spec.Cortex.Enabled), memberlist)
}
