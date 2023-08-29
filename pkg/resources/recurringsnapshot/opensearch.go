package recurringsnapshot

import (
	"strings"
	"time"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	osapi "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearch "github.com/rancher/opni/pkg/opensearch/reconciler"
	"github.com/rancher/opni/pkg/util/meta"
	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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

func (r *Reconciler) reconcileOpensearchPolicy(cluster *opensearchv1.OpenSearchCluster) error {
	reconciler, err := r.createOpensearchReconciler(cluster)
	if err != nil {
		return err
	}

	return reconciler.MaybeUpdateSnapshotPolicy(r.snapshot.Name, r.buildSnapshotPolicy())
}

func (r *Reconciler) buildSnapshotPolicy() osapi.SnapshotManagementRequest {
	return osapi.SnapshotManagementRequest{
		Description: "Snapshot policy created by Kubernetes",
		Enabled:     lo.ToPtr(true),
		SnapshotConfig: osapi.SnapshotConfig{
			DateFormatTimezone: "utc",
			SnapshotRequest: osapi.SnapshotRequest{
				Indices:            strings.Join(r.snapshot.Spec.Snapshot.Indices, ","),
				IgnoreUnavailable:  r.snapshot.Spec.Snapshot.IgnoreUnavailable,
				IncludeGlobalState: r.snapshot.Spec.Snapshot.IncludeGlobalState,
				Partial:            r.snapshot.Spec.Snapshot.AllowPartial,
			},
		},
		Creation: osapi.SnapshotCreation{
			Schedule:  r.snapshot.Spec.Creation.CronSchedule,
			TimeLimit: r.snapshot.Spec.Creation.TimeLimit,
		},
		Deletion: func() *osapi.SnapshotDeletion {
			if r.snapshot.Spec.Retention == nil {
				return nil
			}
			return &osapi.SnapshotDeletion{
				Condition: osapi.SnapshotDeleteCondition{
					MaxCount: r.snapshot.Spec.Retention.MaxCount,
					MaxAge:   r.snapshot.Spec.Retention.MaxAge,
				},
			}
		}(),
	}
}

func (r *Reconciler) updateExecutionStatus(cluster *opensearchv1.OpenSearchCluster) (retResult *reconcile.Result, retErr error) {
	reconciler, retErr := r.createOpensearchReconciler(cluster)
	if retErr != nil {
		return
	}

	if r.snapshot.Status.ExecutionStatus == nil {
		var nextExecution time.Duration
		nextExecution, retErr = reconciler.NextSnapshotPolicyTrigger(r.snapshot.Name)
		if retErr != nil {
			return
		}

		retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.snapshot), r.snapshot); err != nil {
				return err
			}
			r.snapshot.Status.ExecutionStatus = &loggingv1beta1.RecurringSnapshotExecutionStatus{}
			return r.client.Status().Update(r.ctx, r.snapshot)
		})
		if retErr != nil {
			return
		}

		retResult = &reconcile.Result{
			RequeueAfter: nextExecution,
		}
	}

	status, retErr := reconciler.GetSnapshotPolicyLastExecution(r.snapshot.Name)
	if retErr != nil {
		return
	}

	executionStatus := r.snapshot.Status.ExecutionStatus.DeepCopy()
	switch status.Status {
	case osapi.SnapshotPolicyExecutionStatusSuccess:
		var nextExecution time.Duration
		nextExecution, retErr = reconciler.NextSnapshotPolicyTrigger(r.snapshot.Name)
		if retErr != nil {
			return
		}
		executionStatus.LastExecution = metav1.Time(status.EndTime)
		executionStatus.Status = loggingv1beta1.RecurringSnapshotExecutionStateSuccess
		executionStatus.Message = status.Info.Message
		retResult = &reconcile.Result{
			RequeueAfter: nextExecution,
		}
	case osapi.SnapshotPolicyExecutionStatusFailed:
		var nextExecution time.Duration
		nextExecution, retErr = reconciler.NextSnapshotPolicyTrigger(r.snapshot.Name)
		if retErr != nil {
			return
		}
		executionStatus.LastExecution = metav1.Time(status.EndTime)
		executionStatus.Status = loggingv1beta1.RecurringSnapshotExecutionStateFailed
		executionStatus.Message = status.Info.Message
		executionStatus.Cause = status.Info.Cause
		retResult = &reconcile.Result{
			RequeueAfter: nextExecution,
		}
	case osapi.SnapshotPolicyExecutionStatusTimedOut:
		var nextExecution time.Duration
		nextExecution, retErr = reconciler.NextSnapshotPolicyTrigger(r.snapshot.Name)
		if retErr != nil {
			return
		}
		executionStatus.LastExecution = metav1.Time(status.EndTime)
		executionStatus.Status = loggingv1beta1.RecurringSnapshotExecutionStateTimedOut
		executionStatus.Message = status.Info.Message
		executionStatus.Cause = status.Info.Cause
		retResult = &reconcile.Result{
			RequeueAfter: nextExecution,
		}
	case osapi.SnapshotPolicyExecutionStatusInProgress:
		executionStatus.LastExecution = metav1.Now()
		executionStatus.Status = loggingv1beta1.RecurringSnapshotExecutionStateInProgress
		executionStatus.Message = status.Info.Message
		retResult = &reconcile.Result{
			RequeueAfter: 10 * time.Second,
		}
	case osapi.SnapshotPolicyExecutionStatusRetrying:
		executionStatus.LastExecution = metav1.Now()
		executionStatus.Status = loggingv1beta1.RecurringSnapshotExecutionStateRetrying
		executionStatus.Message = status.Info.Message
		executionStatus.Cause = status.Info.Cause
		retResult = &reconcile.Result{
			RequeueAfter: 10 * time.Second,
		}
	}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.snapshot), r.snapshot); err != nil {
			return err
		}
		r.snapshot.Status.ExecutionStatus = executionStatus
		return r.client.Status().Update(r.ctx, r.snapshot)
	})
	if err != nil {
		return nil, err
	}

	return
}

func (r *Reconciler) deleteOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
	if cluster != nil {
		osReconciler, err := r.createOpensearchReconciler(cluster)
		if err != nil {
			return err
		}

		err = osReconciler.MaybeDeleteSnapshotPolicy(r.snapshot.Name)
		if err != nil {
			return err
		}
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.snapshot), r.snapshot); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.snapshot, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.snapshot)
	})
}
