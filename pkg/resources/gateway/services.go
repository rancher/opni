package gateway

import (
	"time"

	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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
			Name:        "opni",
			Namespace:   r.namespace,
			Labels:      publicSvcLabels,
			Annotations: r.spec.ServiceAnnotations,
		},
		Spec: corev1.ServiceSpec{
			Type:     r.spec.ServiceType,
			Selector: resources.NewGatewayLabels(),
			Ports:    servicePorts(publicPorts),
		},
	}

	if r.gw != nil {
		r.gw.Status.ServiceName = publicSvc.Name
	}
	if r.coreGW != nil {
		r.coreGW.Status.ServiceName = publicSvc.Name
	}

	internalPorts, err := r.internalContainerPorts()
	if err != nil {
		return nil, err
	}
	internalSvcLabels := resources.NewGatewayLabels()
	internalSvcLabels["service-type"] = "internal"
	internalSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-internal",
			Namespace: r.namespace,
			Labels:    internalSvcLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: resources.NewGatewayLabels(),
			Ports:    servicePorts(internalPorts),
		},
	}

	adminDashboardPorts, err := r.adminDashboardContainerPorts()
	if err != nil {
		return nil, err
	}
	adminDashboardSvcLabels := resources.NewGatewayLabels()
	adminDashboardSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-admin-dashboard",
			Namespace: r.namespace,
			Labels:    adminDashboardSvcLabels,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: resources.NewGatewayLabels(),
			Ports:    servicePorts(adminDashboardPorts),
		},
	}

	// ensure legacy services are removed
	legacyPublicSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.namespace,
		},
	}
	legacyInternalSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring-internal",
			Namespace: r.namespace,
		},
	}

	r.setOwner(publicSvc)
	r.setOwner(internalSvc)
	r.setOwner(adminDashboardSvc)
	return []resources.Resource{
		resources.Present(publicSvc),
		resources.Present(internalSvc),
		resources.Present(adminDashboardSvc),
		resources.Absent(legacyPublicSvc),
		resources.Absent(legacyInternalSvc),
	}, nil
}

func (r *Reconciler) waitForLoadBalancer() k8sutil.RequeueOp {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni",
			Namespace: r.namespace,
		},
	}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(svc), svc); err != nil {
		return k8sutil.RequeueErr(err)
	}
	if len(svc.Status.LoadBalancer.Ingress) == 0 {
		return k8sutil.Requeue()
	}

	if r.gw != nil {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.gw), r.gw)
			if err != nil {
				return err
			}
			r.gw.Status.LoadBalancer = &svc.Status.LoadBalancer.Ingress[0]
			return r.client.Status().Update(r.ctx, r.gw)
		})
		if err != nil {
			return k8sutil.RequeueErr(err)
		}
	}

	if r.coreGW != nil {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.coreGW), r.coreGW)
			if err != nil {
				return err
			}
			r.coreGW.Status.LoadBalancer = &svc.Status.LoadBalancer.Ingress[0]
			return r.client.Status().Update(r.ctx, r.coreGW)
		})
		if err != nil {
			return k8sutil.RequeueErr(err)
		}
	}

	return k8sutil.DoNotRequeue()
}

func (r *Reconciler) waitForServiceEndpoints() k8sutil.RequeueOp {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni",
			Namespace: r.namespace,
		},
	}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(svc), svc); err != nil {
		return k8sutil.RequeueErr(err)
	}
	endpoints := &corev1.Endpoints{}
	if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(svc), endpoints); err != nil {
		return k8sutil.RequeueErr(err)
	}
	addresses := []corev1.EndpointAddress{}
	for _, subset := range endpoints.Subsets {
		addresses = append(addresses, subset.Addresses...)
	}
	if len(addresses) == 0 {
		if r.gw != nil {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.gw), r.gw)
				if err != nil {
					return err
				}
				r.gw.Status.Endpoints = nil
				return r.client.Status().Update(r.ctx, r.gw)
			})
			if err != nil {
				return k8sutil.RequeueErr(err)
			}
			return k8sutil.RequeueAfter(1 * time.Second)
		}
		if r.coreGW != nil {
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.coreGW), r.coreGW)
				if err != nil {
					return err
				}
				r.coreGW.Status.Endpoints = nil
				return r.client.Status().Update(r.ctx, r.coreGW)
			})
			if err != nil {
				return k8sutil.RequeueErr(err)
			}
			return k8sutil.RequeueAfter(1 * time.Second)
		}
	}

	if r.gw != nil {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.gw), r.gw)
			if err != nil {
				return err
			}
			r.gw.Status.Endpoints = addresses
			return r.client.Status().Update(r.ctx, r.gw)
		})
		if err != nil {
			return k8sutil.RequeueErr(err)
		}
	}
	if r.coreGW != nil {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.coreGW), r.coreGW)
			if err != nil {
				return err
			}
			r.coreGW.Status.Endpoints = addresses
			return r.client.Status().Update(r.ctx, r.coreGW)
		})
		if err != nil {
			return k8sutil.RequeueErr(err)
		}
	}

	return k8sutil.DoNotRequeue()
}
