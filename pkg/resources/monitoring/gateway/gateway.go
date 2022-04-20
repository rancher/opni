package gateway

import (
	"context"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:    ctx,
		client: client,
		mc:     mc,
		logger: logger.New().Named("controller").Named("gateway"),
	}
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	allResources := []resources.Resource{}
	configMap, err := r.configMap()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, configMap)
	certs, err := r.certs()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, certs...)
	deployment, err := r.deployment()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, deployment)
	services, err := r.services()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, services...)
	rbac, err := r.rbac()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, rbac...)
	allResources = append(allResources, r.serviceMonitor())

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.ResultPtr()
	}
	return nil, nil
}
