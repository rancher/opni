package alerting

import (
	"context"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx    context.Context
	client client.Client
	ac     *corev1beta1.AlertingCluster
	gw     *corev1beta1.Gateway
	logger *zap.SugaredLogger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	instance *corev1beta1.AlertingCluster,
) *Reconciler {
	logger := logger.New().Named("controller").Named("alerting")
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(
			client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:    ctx,
		client: client,
		logger: logger,
		ac:     instance,
	}
}

func (r *Reconciler) Reconcile() (reconcile.Result, error) {
	gw := &corev1beta1.Gateway{}
	err := r.client.Get(r.ctx, client.ObjectKey{
		Name:      r.ac.Spec.Gateway.Name,
		Namespace: r.ac.Namespace,
	}, gw)
	if err != nil {
		return k8sutil.RequeueErr(err).Result()
	}
	r.gw = gw

	if gw.DeletionTimestamp != nil {
		return k8sutil.DoNotRequeue().Result()
	}

	allResources := []resources.Resource{}
	// vanilla alertmanager
	allResources = append(allResources, r.alerting()...)

	// cortex alertmanager
	// cortexResources, err := r.cortex()
	// if err != nil {
	// return k8sutil.RequeueErr(err).Result()
	// }
	// allResources = append(allResources, cortexResources...)

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.Result()
	}

	return k8sutil.DoNotRequeue().Result()
}
