package loggingcluster

import (
	"context"
	"errors"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
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
	client         client.Client
	loggingCluster *v1beta2.LoggingCluster
	ctx            context.Context
}

func NewReconciler(
	ctx context.Context,
	loggingCluster *v1beta2.LoggingCluster,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:         c,
		loggingCluster: loggingCluster,
		ctx:            ctx,
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
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
				return err
			}
			r.loggingCluster.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.loggingCluster.Status.State = v1beta2.LoggingClusterStateError
				}
			}
			return r.client.Status().Update(r.ctx, r.loggingCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.loggingCluster.Spec.OpensearchClusterRef == nil {
		retErr = errors.New("logging cluster not provided")
		return
	}

	if r.loggingCluster.Spec.IndexUserSecret == nil {
		retErr = errors.New("index user secret not provided")
		return
	}

	opensearchCluster := &opensearchv1.OpenSearchCluster{}
	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.loggingCluster.Spec.OpensearchClusterRef.Name,
		Namespace: r.loggingCluster.Spec.OpensearchClusterRef.Namespace,
	}, opensearchCluster)
	if retErr != nil {
		return
	}

	// Handle finalizer
	if r.loggingCluster.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.loggingCluster, meta.OpensearchFinalizer) {
		retErr = r.deleteOpensearchObjects(opensearchCluster)
		return
	}

	switch r.loggingCluster.Status.State {
	case "":
		retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
				return err
			}
			r.loggingCluster.Status.State = v1beta2.LoggingClusterStateCreated
			r.loggingCluster.Status.IndexUserState = v1beta2.IndexUserStatePending
			return r.client.Status().Update(r.ctx, r.loggingCluster)
		})
		return
	default:
		_, ok := r.loggingCluster.Labels[resources.OpniClusterID]
		if ok {
			retResult, retErr = r.ReconcileOpensearchUsers(opensearchCluster)
			if retErr != nil {
				return
			}
			retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
					return err
				}
				r.loggingCluster.Status.State = v1beta2.LoggingClusterStateRegistered
				r.loggingCluster.Status.IndexUserState = v1beta2.IndexUserStateCreated
				return r.client.Status().Update(r.ctx, r.loggingCluster)
			})
		}
	}

	return
}
