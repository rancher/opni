package snapshot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cisco-open/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/util"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client   client.Client
	snapshot *loggingv1beta1.Snapshot
	ctx      context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *loggingv1beta1.Snapshot,
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

	repository := &loggingv1beta1.OpensearchRepository{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.snapshot.Spec.Repository.Name,
		Namespace: r.snapshot.Namespace,
	}, repository)

	if err != nil {
		retErr = err
		if k8serrors.IsNotFound(err) {
			lg.V(1).Error(nil, "repository does not exist, not reconciling")
			return nil, nil
		}
		lg.Error(err, "failed to fetch opensearch repository object")
		return
	}

	if repository.Status.State != loggingv1beta1.OpensearchRepositoryCreated {
		lg.Info("waiting for repository to be created")
		return &reconcile.Result{
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	opensearch := &opensearchv1.OpenSearchCluster{}
	if err := r.client.Get(
		r.ctx,
		repository.Spec.OpensearchClusterRef.ObjectKeyFromRef(),
		opensearch,
	); err != nil {
		retErr = err
		if k8serrors.IsNotFound(retErr) {
			lg.V(1).Error(nil, "opensearch cluster does not exist, not reconciling")
			return nil, nil
		}
		lg.Error(err, "failed to fetch opensearch object")
		return
	}

	switch r.snapshot.Status.State {
	case "":
		retResult = &reconcile.Result{
			RequeueAfter: 10 * time.Second,
		}
		retErr = r.generateSnapshotName()
		if err != nil {
			lg.Error(err, "failed to update status")
		}
		return
	case loggingv1beta1.SnapshotStateCreateError, loggingv1beta1.SnapshotStatePending:
		retResult = &reconcile.Result{
			RequeueAfter: 10 * time.Second,
		}
		retErr = r.createOpensearchSnapshot(opensearch)
		return
	case loggingv1beta1.SnapshotStateFetchError, loggingv1beta1.SnapshotStateInProgress:
		requeue, err := r.getSnapshotState(opensearch)
		if err != nil {
			return nil, err
		}
		if requeue {
			retResult = &reconcile.Result{
				RequeueAfter: 10 * time.Second,
			}
		}
		return
	case loggingv1beta1.SnapshotStateCreated:
		return
	default:
		return nil, errors.New("invalid snapshot state")
	}
}

func (r *Reconciler) generateSnapshotName() error {
	suffix := util.GenerateRandomString(6)
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.snapshot), r.snapshot); err != nil {
			return err
		}
		r.snapshot.Status.SnapshotAPIName = fmt.Sprintf("%s-%s", r.snapshot.Name, strings.ToLower(string(suffix)))
		r.snapshot.Status.State = loggingv1beta1.SnapshotStatePending
		return r.client.Status().Update(r.ctx, r.snapshot)
	})
}
