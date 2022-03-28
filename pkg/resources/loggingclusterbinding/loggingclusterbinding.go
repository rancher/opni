package loggingclusterbinding

import (
	"context"
	"errors"
	"time"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client                client.Client
	loggingClusterBinding *v1beta2.LoggingClusterBinding
	ctx                   context.Context
}

func NewReconciler(
	ctx context.Context,
	loggingClusterBinding *v1beta2.LoggingClusterBinding,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:                c,
		loggingClusterBinding: loggingClusterBinding,
		ctx:                   ctx,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the loggingcluster
		// is and set it in the state field accordingly.
		op := util.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingClusterBinding), r.loggingClusterBinding); err != nil {
				return err
			}
			r.loggingClusterBinding.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.loggingClusterBinding.Status.State = v1beta2.LoggingClusterBindingStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.loggingClusterBinding.Status.State = v1beta2.LoggingClusterBindingStateWorking
				}
			} else if len(r.loggingClusterBinding.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.loggingClusterBinding.Status.State = v1beta2.LoggingClusterBindingStateReady
			}
			return r.client.Status().Update(r.ctx, r.loggingClusterBinding)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.loggingClusterBinding.Spec.OpensearchClusterRef == nil {
		retErr = errors.New("opensearch cluster not provided")
		return
	}

	opensearch := &opensearchv1.OpenSearchCluster{}
	if err := r.client.Get(
		r.ctx,
		r.loggingClusterBinding.Spec.OpensearchClusterRef.ObjectKeyFromRef(),
		opensearch,
	); err != nil {
		retErr = err
		return
	}

	if r.loggingClusterBinding.Spec.MulticlusterUser == nil {
		retErr = errors.New("multicluster user must be provided")
		return
	}

	if r.loggingClusterBinding.Spec.LoggingCluster == nil {
		retErr = errors.New("logging cluster must be provided")
		return
	}

	// If the opensearch cluster isn't ready we will immediately requeue
	// TODO add ready state to operator
	if opensearch.Status.Phase != opensearchv1.PhaseRunning {
		lg.Info("opensearch cluster is not ready")
		conditions = append(conditions, "waiting for opensearch")
		retResult = &reconcile.Result{
			RequeueAfter: 5 * time.Second,
		}
		return
	}

	// Handle finalizer
	if r.loggingClusterBinding.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.loggingClusterBinding, meta.OpensearchFinalizer) {
		retErr = r.deleteOpensearchObjects(opensearch)
		return
	}

	retErr = r.reconcileOpensearchObjects(opensearch)

	return
}
