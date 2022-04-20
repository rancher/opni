package cortex

import (
	"context"
	"fmt"

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
		logger: logger.New().Named("controller").Named("cortex"),
	}
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	allResources := []resources.Resource{}
	config, err := r.config()
	if err != nil {
		return nil, err
	}
	allResources = append(allResources, config)

	runtimeConfig := r.runtimeConfig()
	allResources = append(allResources, runtimeConfig)

	fallbackConfig := r.alertmanagerFallbackConfig()
	allResources = append(allResources, fallbackConfig)

	serviceAccount := r.serviceAccount()
	allResources = append(allResources, serviceAccount)

	deployments := r.deployments()
	allResources = append(allResources, deployments...)

	statefulSets := r.statefulSets()
	allResources = append(allResources, statefulSets...)

	services := r.services()
	allResources = append(allResources, services...)

	for _, factory := range allResources {
		o, state, err := factory()
		if err != nil {
			return nil, fmt.Errorf("failed to create object: %w", err)
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			return nil, fmt.Errorf("failed to reconcile resource %#T: %w", o, err)
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, nil
}
