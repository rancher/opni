package multiclusterrolebinding

import (
	"context"
	"fmt"
	"time"

	"github.com/cisco-open/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client                  client.Client
	multiClusterRoleBinding *loggingv1beta1.MulticlusterRoleBinding
	ctx                     context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *loggingv1beta1.MulticlusterRoleBinding,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:                  c,
		multiClusterRoleBinding: instance,
		ctx:                     ctx,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	lg.Info(fmt.Sprintf("tracing feature enabled: %t", features.FeatureList.FeatureIsEnabled("tracing")))

	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := k8sutil.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiClusterRoleBinding), r.multiClusterRoleBinding); err != nil {
				return err
			}
			r.multiClusterRoleBinding.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.multiClusterRoleBinding.Status.State = loggingv1beta1.MulticlusterRoleBindingStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.multiClusterRoleBinding.Status.State = loggingv1beta1.MulticlusterRoleBindingStateWorking
				}
			} else if len(r.multiClusterRoleBinding.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.multiClusterRoleBinding.Status.State = loggingv1beta1.MulticlusterRoleBindingStateReady
			}
			return r.client.Status().Update(r.ctx, r.multiClusterRoleBinding)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	result := reconciler.CombinedResult{}

	if r.multiClusterRoleBinding.Spec.OpensearchCluster == nil {
		return retResult, fmt.Errorf("missing opensearch cluster ref")
	}

	opensearchCluster := &opensearchv1.OpenSearchCluster{}
	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.multiClusterRoleBinding.Spec.OpensearchCluster.Name,
		Namespace: r.multiClusterRoleBinding.Spec.OpensearchCluster.Namespace,
	}, opensearchCluster)
	if retErr != nil {
		result.Combine(&reconcile.Result{}, retErr)
	}

	// If the opensearch cluster isn't ready we will immediately requeue
	// TODO add ready state to operator
	if opensearchCluster.Status.Phase != opensearchv1.PhaseRunning {
		lg.Info("opensearch cluster is not ready")
		conditions = append(conditions, "waiting for opensearch")
		result.Combine(&reconcile.Result{
			RequeueAfter: 5 * time.Second,
		}, nil)

		retResult = &result.Result
		retErr = result.Err
		return
	}

	result.Combine(r.ReconcileOpensearchObjects(opensearchCluster))

	retResult = &result.Result
	retErr = result.Err
	return
}
