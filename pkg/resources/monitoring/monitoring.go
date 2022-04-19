package monitoring

import (
	"context"
	"fmt"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/monitoring/gateway"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx    context.Context
	client client.Client
	mc     *v1beta2.MonitoringCluster
	logger *zap.SugaredLogger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	mc *v1beta2.MonitoringCluster,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client),
		ctx:                ctx,
		client:             client,
		mc:                 mc,
		logger:             logger.New().Named("controller").Named("monitoring"),
	}
}

func (r *Reconciler) Reconcile() (reconcile.Result, error) {
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

	grafanaResources, err := r.grafana()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, grafanaResources...)

	for _, factory := range allResources {
		o, state, err := factory()
		if err != nil {
			return util.RequeueErr(fmt.Errorf("failed to create object: %w", err)).Result()
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			return util.RequeueErr(fmt.Errorf("failed to reconcile resource %#T: %w", o, err)).Result()
		}
		if result != nil {
			return util.LoadResult(result, err).Result()
		}
	}

	gatewayRec := gateway.NewReconciler(r.ctx, r.client, r.mc)
	gatewayResult, err := gatewayRec.Reconcile()
	if err != nil {
		result := util.LoadResult(gatewayResult, err)
		if result.ShouldRequeue() {
			return result.Result()
		}
	}

	return util.DoNotRequeue().Result()
}
