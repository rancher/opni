package gateway

import (
	"context"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx    context.Context
	client client.Client
	gw     *v1beta2.Gateway
	logger *zap.SugaredLogger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	gw *v1beta2.Gateway,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:    ctx,
		client: client,
		gw:     gw,
		logger: logger.New().Named("controller").Named("gateway"),
	}
}

func (r *Reconciler) Reconcile() (retResult reconcile.Result, retErr error) {
	updated, err := r.updateImageStatus()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	if updated {
		return util.Requeue().Result()
	}

	allResources := []resources.Resource{}
	etcdResources, err := r.etcd()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, etcdResources...)
	configMap, err := r.configMap()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, configMap)
	certs, err := r.certs()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, certs...)
	deployment, err := r.deployment()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, deployment)
	services, err := r.services()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, services...)
	rbac, err := r.rbac()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, rbac...)
	allResources = append(allResources, r.serviceMonitor())

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.Result()
	}

	// Post-reconcile, wait for the public service's load balancer to be ready
	if op := r.waitForServiceEndpoints(); op.ShouldRequeue() {
		return op.Result()
	}
	if r.gw.Spec.ServiceType == corev1.ServiceTypeLoadBalancer {
		if op := r.waitForLoadBalancer(); op.ShouldRequeue() {
			return op.Result()
		}
	}

	r.gw.Status.Ready = true
	if err := r.client.Status().Update(r.ctx, r.gw); err != nil {
		return util.RequeueErr(err).Result()
	}

	return util.DoNotRequeue().Result()
}
