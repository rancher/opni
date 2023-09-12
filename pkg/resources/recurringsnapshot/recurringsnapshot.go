package recurringsnapshot

import (
	"context"
	"time"

	"github.com/cisco-open/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/util/meta"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	client   client.Client
	snapshot *loggingv1beta1.RecurringSnapshot
	ctx      context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *loggingv1beta1.RecurringSnapshot,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:   c,
		snapshot: instance,
		ctx:      ctx,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	lg.Info("beginning reccuring snapshot reconciliation")

	defer func() {
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.snapshot), r.snapshot); err != nil {
				return err
			}
			if retErr != nil {
				// If an error occurred, the state should be set to error
				r.snapshot.Status.State = loggingv1beta1.RecurringSnapshotStateError
			} else {
				r.snapshot.Status.State = loggingv1beta1.RecurringSnapshotStateCreated
			}
			return r.client.Status().Update(r.ctx, r.snapshot)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	opensearch := &opensearchv1.OpenSearchCluster{}
	repository := &loggingv1beta1.OpensearchRepository{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.snapshot.Spec.Snapshot.Repository.Name,
		Namespace: r.snapshot.Namespace,
	}, repository)

	if err != nil {
		retErr = err
		if !k8serrors.IsNotFound(err) {
			return
		}
		opensearch = nil
	}

	if err := r.client.Get(
		r.ctx,
		repository.Spec.OpensearchClusterRef.ObjectKeyFromRef(),
		opensearch,
	); err != nil {
		retErr = err
		if !k8serrors.IsNotFound(retErr) {
			return
		}
		opensearch = nil
	}

	// Handle finalizer
	if r.snapshot.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.snapshot, meta.OpensearchFinalizer) {
		retErr = r.deleteOpensearchObjects(opensearch)
		return
	}

	//Once we've checked for deletion, if the opensearch is nil we can return the previous error
	if opensearch == nil {
		return
	}

	if repository.Status.State != loggingv1beta1.OpensearchRepositoryCreated {
		lg.Info("waiting for repository to be created")
		return &reconcile.Result{
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	retErr = r.reconcileOpensearchPolicy(opensearch)
	if retErr != nil {
		return
	}

	return r.updateExecutionStatus(opensearch)
}
