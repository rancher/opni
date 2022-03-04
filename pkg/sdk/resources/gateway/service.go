package gateway

import (
	"github.com/rancher/opni-monitoring/pkg/sdk/resources"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) service() (resources.Resource, error) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-gateway",
			Namespace: r.gateway.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: resources.Labels(),
			Ports: []corev1.ServicePort{
				{
					Name:     "http",
					Protocol: corev1.ProtocolTCP,
					Port:     8080,
				},
			},
		},
	}

	ports, err := r.optionalServicePorts()
	if err != nil {
		return nil, err
	}
	svc.Spec.Ports = append(svc.Spec.Ports, ports...)
	ctrl.SetControllerReference(r.gateway, svc, r.client.Scheme())
	return resources.Present(svc), nil
}
