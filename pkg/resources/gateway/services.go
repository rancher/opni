package gateway

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) services() ([]resources.Resource, error) {
	publicPorts, err := r.publicContainerPorts()
	if err != nil {
		return nil, err
	}
	publicSvcLabels := resources.NewGatewayLabels()
	publicSvcLabels["service-type"] = "public"
	publicSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.gw.Namespace,
			Labels:    publicSvcLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.gw.Spec.ServiceType,
			Selector: resources.NewGatewayLabels(),
			Ports:    servicePorts(publicPorts),
		},
	}

	internalPorts, err := r.managementContainerPorts()
	if err != nil {
		return nil, err
	}
	internalSvcLabels := resources.NewGatewayLabels()
	internalSvcLabels["service-type"] = "internal"
	internalSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring-internal",
			Namespace: r.gw.Namespace,
			Labels:    internalSvcLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: resources.NewGatewayLabels(),
			Ports:    servicePorts(internalPorts),
		},
	}
	ctrl.SetControllerReference(r.gw, publicSvc, r.client.Scheme())
	ctrl.SetControllerReference(r.gw, internalSvc, r.client.Scheme())
	return []resources.Resource{
		resources.Present(publicSvc),
		resources.Present(internalSvc),
	}, nil
}
