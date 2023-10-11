package alerting

import (
	"context"

	"log/slog"

	"github.com/cisco-open/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
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
	lg     *slog.Logger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	instance *corev1beta1.AlertingCluster,
) *Reconciler {
	logger := logger.New().WithGroup("controller").WithGroup("alerting")
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
		lg:     logger,
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
	allResources = append(allResources, r.alerting()...)

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.Result()
	}
	return k8sutil.DoNotRequeue().Result()
}
