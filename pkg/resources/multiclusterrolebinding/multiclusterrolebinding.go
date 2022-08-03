package multiclusterrolebinding

import (
	"context"
	"fmt"
	"time"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client                  client.Client
	multiClusterRoleBinding *v1beta2.MulticlusterRoleBinding
	ctx                     context.Context
}

func NewReconciler(
	ctx context.Context,
	multiClusterRoleBinding *v1beta2.MulticlusterRoleBinding,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:                  c,
		multiClusterRoleBinding: multiClusterRoleBinding,
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
		op := util.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiClusterRoleBinding), r.multiClusterRoleBinding); err != nil {
				return err
			}
			r.multiClusterRoleBinding.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.multiClusterRoleBinding.Status.State = v1beta2.MulticlusterRoleBindingStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.multiClusterRoleBinding.Status.State = v1beta2.MulticlusterRoleBindingStateWorking
				}
			} else if len(r.multiClusterRoleBinding.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.multiClusterRoleBinding.Status.State = v1beta2.MulticlusterRoleBindingStateReady
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

	if opensearchCluster == nil {
		lg.Info("no opensearch cluster, requeueing")
		retResult = &reconcile.Result{RequeueAfter: time.Second * 5}
		retErr = result.Err
		return
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

	// Handle finalizer
	if r.multiClusterRoleBinding.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.multiClusterRoleBinding, meta.OpensearchFinalizer) {
		result.CombineErr(r.deleteOpensearchObjects(opensearchCluster))
		retResult = &result.Result
		retErr = result.Err
		return
	}

	result.Combine(r.ReconcileOpensearchObjects(opensearchCluster))

	retResult = &result.Result
	retErr = result.Err
	return
}
