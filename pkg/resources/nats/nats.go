package nats

import (
	"context"
	"fmt"

	emperrors "emperror.dev/errors"
	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx         context.Context
	client      client.Client
	natsCluster *opnicorev1beta1.NatsCluster
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	natsCluster *opnicorev1beta1.NatsCluster,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			append(
				opts,
				reconciler.WithPatchCalculateOptions(patch.IgnoreVolumeClaimTemplateTypeMetaAndStatus(), patch.IgnoreStatusFields()),
				reconciler.WithLog(log.FromContext(ctx)),
			)...),
		ctx:         ctx,
		client:      client,
		natsCluster: natsCluster,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)

	defer func() {
		// When the reconciler is done, figure out what the state of the natscluster
		// is and set it in the state field accordingly.
		op := k8sutil.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {

			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.natsCluster), r.natsCluster); err != nil {
				return err
			}
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.natsCluster.Status.State = opnicorev1beta1.NatsClusterStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.natsCluster.Status.State = opnicorev1beta1.NatsClusterStateWorking
				}
			} else {
				r.natsCluster.Status.State = opnicorev1beta1.NatsClusterStateReady
			}
			return r.client.Status().Update(r.ctx, r.natsCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	resources, err := r.nats()
	if err != nil {
		retErr = emperrors.Combine(retErr, err)
		lg.Error(err, "Error when reconciling nats, cannot continue.")
		return
	}

	for _, factory := range resources {
		o, state, err := factory()
		if err != nil {
			retErr = emperrors.WrapIf(err, "failed to create object")
			return
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			retErr = emperrors.WrapWithDetails(err, "failed to reconcile resource",
				"resource", o.GetObjectKind().GroupVersionKind())
			return
		}
		if result != nil {
			retResult = result
		}
	}

	return
}
