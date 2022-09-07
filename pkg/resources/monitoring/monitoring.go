package monitoring

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/monitoring/cortex"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx               context.Context
	client            client.Client
	mc                *v1beta2.MonitoringCluster
	gw                *v1beta2.Gateway
	coremc            *corev1beta1.MonitoringCluster
	coregw            *corev1beta1.Gateway
	instanceName      string
	instanceNamespace string
	spec              corev1beta1.MonitoringClusterSpec
	logger            *zap.SugaredLogger
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	instance interface{},
) (*Reconciler, error) {
	logger := logger.New().Named("controller").Named("monitoring")
	r := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			reconciler.WithEnableRecreateWorkload(),
			reconciler.WithRecreateErrorMessageCondition(reconciler.MatchImmutableErrorMessages),
			reconciler.WithLog(log.FromContext(ctx)),
			reconciler.WithScheme(client.Scheme()),
		),
		ctx:    ctx,
		client: client,
		logger: logger,
	}

	switch mc := instance.(type) {
	case *v1beta2.MonitoringCluster:
		r.instanceName = mc.Name
		r.instanceNamespace = mc.Namespace
		r.spec = convertSpec(mc.Spec)
		r.mc = mc
	case *corev1beta1.MonitoringCluster:
		r.instanceName = mc.Name
		r.instanceNamespace = mc.Namespace
		r.spec = mc.Spec
		r.coremc = mc
	default:
		return nil, errors.New("invalid monitoringcluster instance type")
	}

	return r, nil
}

func (r *Reconciler) Reconcile() (reconcile.Result, error) {
	// Look up referenced gateway
	if r.mc != nil {
		gw := &v1beta2.Gateway{}
		err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.mc.Spec.Gateway.Name,
			Namespace: r.mc.Namespace,
		}, gw)
		if err != nil {
			return util.RequeueErr(err).Result()
		}
		r.gw = gw
	}
	if r.coremc != nil {
		gw := &corev1beta1.Gateway{}
		err := r.client.Get(r.ctx, types.NamespacedName{
			Name:      r.mc.Spec.Gateway.Name,
			Namespace: r.mc.Namespace,
		}, gw)
		if err != nil {
			return util.RequeueErr(err).Result()
		}
		r.coregw = gw
	}

	updated, err := r.updateImageStatus()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	if updated {
		return util.Requeue().Result()
	}

	allResources := []resources.Resource{}

	grafanaResources, err := r.grafana()
	if err != nil {
		return util.RequeueErr(err).Result()
	}
	allResources = append(allResources, grafanaResources...)

	if op := resources.ReconcileAll(r, allResources); op.ShouldRequeue() {
		return op.Result()
	}

	cortexRec := cortex.NewReconciler(
		r.ctx,
		r.client,
		r.mc,
		r.spec,
		r.instanceName,
		r.instanceNamespace,
	)
	cortexResult, err := cortexRec.Reconcile()
	if err != nil {
		result := util.LoadResult(cortexResult, err)
		if result.ShouldRequeue() {
			return result.Result()
		}
	}

	return util.DoNotRequeue().Result()
}

func convertSpec(spec v1beta2.MonitoringClusterSpec) corev1beta1.MonitoringClusterSpec {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	retSpec := corev1beta1.MonitoringClusterSpec{}
	err = json.Unmarshal(data, &retSpec)
	if err != nil {
		panic(err)
	}
	return retSpec
}
