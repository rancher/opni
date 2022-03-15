package multiclusteruser

import (
	"context"
	"errors"
	"time"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client           client.Client
	multiclusterUser *v2beta1.MulticlusterUser
	ctx              context.Context
}

func NewReconciler(
	ctx context.Context,
	multiclusterUser *v2beta1.MulticlusterUser,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:           c,
		multiclusterUser: multiclusterUser,
		ctx:              ctx,
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
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiclusterUser), r.multiclusterUser); err != nil {
				return err
			}
			r.multiclusterUser.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.multiclusterUser.Status.State = v2beta1.MulticlusterUserStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.multiclusterUser.Status.State = v2beta1.MulticlusterUserStatePending
				}
			} else if len(r.multiclusterUser.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.multiclusterUser.Status.State = v2beta1.MulticlusterUserStateCreated
			}
			return r.client.Status().Update(r.ctx, r.multiclusterUser)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.multiclusterUser.Spec.Password == "" {
		retErr = errors.New("must set password")
		return
	}

	opensearch := &opensearchv1.OpenSearchCluster{}
	if err := r.client.Get(
		r.ctx,
		r.multiclusterUser.Spec.OpensearchClusterRef.ObjectKeyFromRef(),
		opensearch,
	); err != nil {
		retErr = err
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

	retErr = r.reconcileOpensearchObjects(opensearch)

	return
}
