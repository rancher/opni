package cortex

import (
	"context"
	"errors"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx       context.Context
	client    client.Client
	spec      corev1beta1.MonitoringClusterSpec
	mc        interface{}
	name      string
	namespace string
	logger    *zap.SugaredLogger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	mc interface{},
	spec corev1beta1.MonitoringClusterSpec,
	name string,
	namespace string,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:       ctx,
		client:    client,
		mc:        mc,
		spec:      spec,
		name:      name,
		namespace: namespace,
		logger:    logger.New().Named("controller").Named("cortex"),
	}

}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	allResources := []resources.Resource{}

	updated, err := r.updateCortexVersionStatus()
	if err != nil {
		return util.RequeueErr(err).ResultPtr()
	}
	if updated {
		return util.Requeue().ResultPtr()
	}

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

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.ResultPtr()
	}

	// watch cortex components until they are healthy
	if op := r.pollCortexHealth(append(deployments, statefulSets...)); op.ShouldRequeue() {
		return op.ResultPtr()
	}

	return nil, nil
}

func (r *Reconciler) setOwner(obj client.Object) error {
	switch cluster := r.mc.(type) {
	case *v1beta2.MonitoringCluster:
		err := ctrl.SetControllerReference(cluster, obj, r.client.Scheme())
		if err != nil {
			return err
		}
	case *corev1beta1.MonitoringCluster:
		err := ctrl.SetControllerReference(cluster, obj, r.client.Scheme())
		if err != nil {
			return err
		}
	default:
		return errors.New("unsupported monitoring type")
	}

	return nil
}
