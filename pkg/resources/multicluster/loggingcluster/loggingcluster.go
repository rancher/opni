package loggingcluster

import (
	"context"
	"time"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	opensearchv1beta1 "github.com/rancher/opni-opensearch-operator/api/v1beta1"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources/multicluster"
	"github.com/rancher/opni/pkg/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client         client.Client
	loggingCluster *v2beta1.LoggingCluster
	ctx            context.Context
}

func NewReconciler(
	ctx context.Context,
	loggingCluster *v2beta1.LoggingCluster,
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
		// When the reconciler is done, figure out what the state of the opnicluster
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
					r.loggingCluster.Status.State = v2beta1.LoggingClusterStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.loggingCluster.Status.State = v2beta1.LoggingClusterStateWorking
				}
			} else if len(r.loggingCluster.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.loggingCluster.Status.State = v2beta1.LoggingClusterStateReady
			}
			return r.client.Status().Update(r.ctx, r.loggingCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	result := reconciler.CombinedResult{}

	defaultCluster := r.buildDefaultOpensearchCluster()

	if r.loggingCluster.Spec.OpensearchCluster == nil {
		result.Combine(r.ReconcileResource(defaultCluster, reconciler.StatePresent))
	} else {
		result.Combine(r.ReconcileResource(defaultCluster, reconciler.StateAbsent))
	}

	opensearchCluster, err := multicluster.FetchOpensearchCluster(r.ctx, r.client, r.loggingCluster)
	if err != nil {
		result.Combine(&reconcile.Result{}, err)
	}

	if opensearchCluster == nil {
		lg.Info("no opensearch cluster, requeueing")
		retResult = &reconcile.Result{RequeueAfter: time.Second * 5}
		retErr = result.Err
		return
	}

	// If the opensearch cluster isn't ready we will immediately requeue
	if opensearchCluster.Status.State != opensearchv1beta1.OpensearchClusterStateReady {
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

func (r *Reconciler) buildDefaultOpensearchCluster() *opensearchv1beta1.OpensearchCluster {
	cluster := &opensearchv1beta1.OpensearchCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      multicluster.OpensearchClusterNameDefault,
			Namespace: r.loggingCluster.Namespace,
		},
		Spec: opensearchv1beta1.OpensearchClusterSpec{
			Version: "1.1.0",
			Master: opensearchv1beta1.OpensearchWorkloadOptions{
				Replicas: pointer.Int32(3),
			},
			Data: opensearchv1beta1.OpensearchWorkloadOptions{
				Replicas: pointer.Int32(2),
			},
		},
	}
	ctrl.SetControllerReference(r.loggingCluster, cluster, r.client.Scheme())

	return cluster
}
