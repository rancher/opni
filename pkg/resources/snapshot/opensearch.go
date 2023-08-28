package snapshot

import (
	"strings"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	osapi "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearch "github.com/rancher/opni/pkg/opensearch/reconciler"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *Reconciler) createOpensearchReconciler(cluster *opensearchv1.OpenSearchCluster) (*opensearch.Reconciler, error) {
	certMgr := certs.NewCertMgrOpensearchCertManager(
		r.ctx,
		certs.WithNamespace(cluster.Namespace),
		certs.WithCluster(cluster.Name),
	)

	return opensearch.NewReconciler(
		r.ctx,
		opensearch.ReconcilerConfig{
			CertReader:            certMgr,
			OpensearchServiceName: cluster.Spec.General.ServiceName,
		},
	)
}

func (r *Reconciler) createOpensearchSnapshot(cluster *opensearchv1.OpenSearchCluster) (retErr error) {
	// calculate state
	defer func() {
		state := loggingv1beta1.SnapshotStateInProgress
		lg := log.FromContext(r.ctx)
		if retErr != nil {
			state = loggingv1beta1.SnapshotStateCreateError
		}
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.snapshot), r.snapshot); err != nil {
				return err
			}
			if retErr != nil {
				r.snapshot.Status.FailureMessage = retErr.Error()
			}
			r.snapshot.Status.State = state
			return r.client.Status().Update(r.ctx, r.snapshot)
		})
		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	reconciler, retErr := r.createOpensearchReconciler(cluster)
	if retErr != nil {
		return
	}

	retErr = reconciler.CreateSnapshotAsync(
		r.snapshot.Status.SnapshotAPIName,
		r.snapshot.Spec.Repository.Name,
		osapi.SnapshotRequest{
			Indices:            strings.Join(r.snapshot.Spec.Indices, ","),
			IgnoreUnavailable:  r.snapshot.Spec.IgnoreUnavailable,
			IncludeGlobalState: r.snapshot.Spec.IncludeGlobalState,
			Partial:            r.snapshot.Spec.AllowPartial,
		},
	)
	return
}

func (r *Reconciler) getSnapshotState(cluster *opensearchv1.OpenSearchCluster) (requeue bool, retErr error) {
	state := loggingv1beta1.SnapshotStateInProgress
	failureMessage := ""
	defer func() {
		lg := log.FromContext(r.ctx)
		if retErr != nil {
			state = loggingv1beta1.SnapshotStateFetchError
		}
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.snapshot), r.snapshot); err != nil {
				return err
			}

			r.snapshot.Status.FailureMessage = failureMessage
			r.snapshot.Status.State = state
			return r.client.Status().Update(r.ctx, r.snapshot)
		})
		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	reconciler, retErr := r.createOpensearchReconciler(cluster)
	if retErr != nil {
		return
	}

	snapshotState, failureMessage, retErr := reconciler.GetSnapshotState(
		r.snapshot.Status.SnapshotAPIName,
		r.snapshot.Spec.Repository.Name,
	)

	switch snapshotState {
	case osapi.SnapshotStateSuccess:
		state = loggingv1beta1.SnapshotStateCreated
	case osapi.SnapshotStatePartial:
		state = loggingv1beta1.SnapshotStateFailedPartial
	case osapi.SnapshotStateFailed:
		state = loggingv1beta1.SnapshotStateFailed
	default:
		state = loggingv1beta1.SnapshotStateInProgress
		requeue = true
	}

	return
}
