package monitoring

import (
	"context"

	"github.com/cisco-open/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/monitoring/cortex"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx    context.Context
	client client.Client
	mc     *corev1beta1.MonitoringCluster
	gw     *corev1beta1.Gateway
	logger *zap.SugaredLogger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	instance *corev1beta1.MonitoringCluster,
) *Reconciler {
	logger := logger.NewZap().Named("controller").Named("monitoring")
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:    ctx,
		client: client,
		logger: logger,
		mc:     instance,
	}
}

func (r *Reconciler) Reconcile() (reconcile.Result, error) {
	gw := &corev1beta1.Gateway{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.mc.Spec.Gateway.Name,
		Namespace: r.mc.Namespace,
	}, gw)
	if err != nil {
		return k8sutil.RequeueErr(err).Result()
	}
	r.gw = gw

	if gw.DeletionTimestamp != nil {
		return k8sutil.DoNotRequeue().Result()
	}

	updated, err := r.updateImageStatus()
	if err != nil {
		return k8sutil.RequeueErr(err).Result()
	}
	if updated {
		return k8sutil.Requeue().Result()
	}

	allResources := []resources.Resource{}

	grafanaResources, err := r.grafana()
	if err != nil {
		return k8sutil.RequeueErr(err).Result()
	}
	allResources = append(allResources, grafanaResources...)

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.Result()
	}

	cortexRec := cortex.NewReconciler(
		r.ctx,
		r.client,
		r.mc,
	)
	cortexResult, err := cortexRec.Reconcile()
	if err != nil {
		result := k8sutil.LoadResult(cortexResult, err)
		if result.ShouldRequeue() {
			return result.Result()
		}
	}

	return k8sutil.DoNotRequeue().Result()
}
