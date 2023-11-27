package opensearchrepository

import (
	"context"
	"time"

	opensearchv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	"github.com/cisco-open/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/util/meta"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client      client.Client
	respository *loggingv1beta1.OpensearchRepository
	ctx         context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *loggingv1beta1.OpensearchRepository,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:      c,
		respository: instance,
		ctx:         ctx,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)

	defer func() {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.respository), r.respository); err != nil {
				return err
			}
			if retErr != nil {
				// If an error occurred, the state should be set to error
				r.respository.Status.State = loggingv1beta1.OpensearchRepositoryError
				r.respository.Status.FailureMessage = retErr.Error()
			} else {
				r.respository.Status.State = loggingv1beta1.OpensearchRepositoryCreated
			}
			return r.client.Status().Update(r.ctx, r.respository)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	opensearch := &opensearchv1.OpenSearchCluster{}
	if err := r.client.Get(
		r.ctx,
		r.respository.Spec.OpensearchClusterRef.ObjectKeyFromRef(),
		opensearch,
	); err != nil {
		retErr = err
		if !k8serrors.IsNotFound(retErr) {
			return
		}
		// Set opensearch to nil if it doesn't exist for finalizer
		opensearch = nil
	}

	// Handle finalizer
	if r.respository.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.respository, meta.OpensearchFinalizer) {
		retErr = r.deleteOpensearchObjects(opensearch)
		return
	}

	//Once we've checked for deletion, if the opensearch is nil we can return the previous error
	if opensearch == nil {
		return
	}

	if opensearch.Status.Phase != opensearchv1.PhaseRunning {
		lg.Info("opensearch cluster is not ready")
		retResult = &reconcile.Result{
			RequeueAfter: 5 * time.Second,
		}
		return
	}

	retErr = r.reconcileOpensearchObjects(opensearch)

	return
}
