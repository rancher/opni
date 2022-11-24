package opnicluster

import (
	"context"
	"fmt"
	"time"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	prometheusRuleFinalizer = "opni.io/prometheusRule"
)

var (
	ErrOpensearchUpgradeFailed = errors.New("opensearch upgrade failed")
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx               context.Context
	client            client.Client
	recorder          record.EventRecorder
	opniCluster       *aiv1beta1.OpniCluster
	opensearchCluster *opensearchv1.OpenSearchCluster
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	recorder record.EventRecorder,
	instance *aiv1beta1.OpniCluster,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		ctx:         ctx,
		client:      client,
		recorder:    recorder,
		opniCluster: instance,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := k8sutil.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
				return err
			}
			r.opniCluster.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.opniCluster.Status.State = aiv1beta1.OpniClusterStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.opniCluster.Status.State = aiv1beta1.OpniClusterStateWorking
				}
			} else if len(r.opniCluster.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.opniCluster.Status.State = aiv1beta1.OpniClusterStateReady
			}
			return r.client.Status().Update(r.ctx, r.opniCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	// Handle finalizer
	if r.opniCluster.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.opniCluster, prometheusRuleFinalizer) {
		retResult, retErr = r.cleanupPrometheusRule()
		if retErr != nil || retResult != nil {
			return
		}
	}

	r.opensearchCluster = &opensearchv1.OpenSearchCluster{}
	err := r.client.Get(r.ctx, r.opniCluster.Spec.Opensearch.ObjectKeyFromRef(), r.opensearchCluster)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
		lg.V(1).Error(err, "external opensearch object does not exist")
	}

	allResources := []resources.Resource{}
	opniServices, err := r.opniServices()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		return nil, err
	}
	pretrained, err := r.pretrainedModels()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling pretrained models, will retry.")
		// Keep going, we can reconcile the rest of the deployments and come back
		// to this later.
	}

	workloadDrain, err := r.workloadDrain()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling workload drain, will retry.")
	}

	var es []resources.Resource
	if r.opensearchCluster == nil {
		lg.Error(err, "external opensearch not found")
		return
	}
	es, err = r.externalOpensearchConfig()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when setting external opensearch config")
		return
	}

	var s3 []resources.Resource
	intS3, err := r.internalS3()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling s3, cannot continue.")
		return
	}
	extS3, err := r.externalS3()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling s3, cannot continue.")
		return
	}
	s3 = append(s3, intS3...)
	s3 = append(s3, extS3...)

	// Order is important here
	// nats, s3, and elasticsearch reconcilers will add fields to the opniCluster status object
	// which are used by other reconcilers.
	allResources = append(allResources, s3...)
	allResources = append(allResources, es...)
	allResources = append(allResources, opniServices...)
	allResources = append(allResources, workloadDrain...)
	allResources = append(allResources, pretrained...)

	for _, factory := range allResources {
		o, state, err := factory()
		if err != nil {
			retErr = errors.WrapIf(err, "failed to create object")
			return
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			retErr = errors.WrapWithDetails(err, "failed to reconcile resource",
				"resource", o.GetObjectKind().GroupVersionKind())
			return
		}
		if result != nil {
			retResult = result
		}
	}

	if err != nil {
		return nil, err
	}

	return
}

func (r *Reconciler) cleanupPrometheusRule() (retResult *reconcile.Result, retErr error) {
	namespace := r.opniCluster.Status.PrometheusRuleNamespace
	if namespace == "" {
		retErr = errors.New("prometheusRule namespace is unknown")
		return
	}
	prometheusRule := &monitoringv1.PrometheusRule{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-%s", aiv1beta1.MetricsService.ServiceName(), r.generateSHAID()),
		Namespace: namespace,
	}, prometheusRule)
	if k8serrors.IsNotFound(err) {
		retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
				return err
			}
			controllerutil.RemoveFinalizer(r.opniCluster, prometheusRuleFinalizer)
			return r.client.Update(r.ctx, r.opniCluster)
		})
		return
	} else if err != nil {
		retErr = err
		return
	}
	retErr = r.client.Delete(r.ctx, prometheusRule)
	if retErr != nil {
		return
	}
	return &reconcile.Result{RequeueAfter: time.Second}, nil
}

func RegisterWatches(builder *builder.Builder) *builder.Builder {
	return builder
}
