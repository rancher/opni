package gateway

import (
	"time"

	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			Name:        "opni-monitoring",
			Namespace:   r.gw.Namespace,
			Labels:      publicSvcLabels,
			Annotations: r.gw.Spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.gw.Spec.ServiceType,
			Selector: resources.NewGatewayLabels(),
			Ports:    servicePorts(publicPorts),
		},
	}

	r.gw.Status.ServiceName = publicSvc.Name

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

func (r *Reconciler) waitForLoadBalancer() util.RequeueOp {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.gw.Namespace,
		},
	}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(svc), svc); err != nil {
		return util.RequeueErr(err)
	}
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return util.Requeue()
	}
	r.gw.Status.LoadBalancer = &svc.Status.LoadBalancer.Ingress[0]

	if err := r.client.Status().Update(r.ctx, r.gw); err != nil {
		return util.RequeueErr(err)
	}
	return util.DoNotRequeue()
}

func (r *Reconciler) waitForServiceEndpoints() util.RequeueOp {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.gw.Namespace,
		},
	}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(svc), svc); err != nil {
		return util.RequeueErr(err)
	}
	endpoints := &corev1.Endpoints{}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(svc), endpoints); err != nil {
		return util.RequeueErr(err)
	}
	addresses := []corev1.EndpointAddress{}
	for _, subset := range endpoints.Subsets {
		addresses = append(addresses, subset.Addresses...)
	}
	if len(addresses) == 0 {
		r.gw.Status.Endpoints = nil
		if err := r.client.Status().Update(r.ctx, r.gw); err != nil {
			return util.RequeueErr(err)
		}
		return util.RequeueAfter(1 * time.Second)
	}
	r.gw.Status.Endpoints = addresses
	if err := r.client.Status().Update(r.ctx, r.gw); err != nil {
		return util.RequeueErr(err)
	}
	return util.DoNotRequeue()
}
