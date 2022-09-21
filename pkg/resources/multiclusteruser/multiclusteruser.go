package multiclusteruser

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/util/k8sutil"
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
	client            client.Client
	multiclusterUser  *v1beta2.MulticlusterUser
	loggingMCU        *loggingv1beta1.MulticlusterUser
	instanceName      string
	instanceNamespace string
	spec              loggingv1beta1.MulticlusterUserSpec
	ctx               context.Context
}

func NewReconciler(
	ctx context.Context,
	instance interface{},
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) (*Reconciler, error) {
	r := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client: c,
		ctx:    ctx,
	}

	switch mcu := instance.(type) {
	case *v1beta2.MulticlusterUser:
		r.instanceName = mcu.Name
		r.instanceNamespace = mcu.Namespace
		r.spec = convertSpec(mcu.Spec)
		r.multiclusterUser = mcu
	case *loggingv1beta1.MulticlusterUser:
		r.instanceName = mcu.Name
		r.instanceNamespace = mcu.Namespace
		r.spec = mcu.Spec
		r.loggingMCU = mcu
	default:
		return nil, errors.New("invalid multiclusteruser instance type")
	}
	return r, nil
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the loggingcluster
		// is and set it in the state field accordingly.
		op := k8sutil.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if r.multiclusterUser != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiclusterUser), r.multiclusterUser); err != nil {
					return err
				}
				r.multiclusterUser.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.multiclusterUser.Status.State = v1beta2.MulticlusterUserStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.multiclusterUser.Status.State = v1beta2.MulticlusterUserStatePending
					}
				} else if len(r.multiclusterUser.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.multiclusterUser.Status.State = v1beta2.MulticlusterUserStateCreated
				}
				return r.client.Status().Update(r.ctx, r.multiclusterUser)
			}
			if r.loggingMCU != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingMCU), r.loggingMCU); err != nil {
					return err
				}
				r.loggingMCU.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.loggingMCU.Status.State = loggingv1beta1.MulticlusterUserStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.loggingMCU.Status.State = loggingv1beta1.MulticlusterUserStatePending
					}
				} else if len(r.loggingMCU.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.loggingMCU.Status.State = loggingv1beta1.MulticlusterUserStateCreated
				}
				return r.client.Status().Update(r.ctx, r.multiclusterUser)
			}
			return errors.New("no multiclusteruser instance to update")
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.spec.Password == "" {
		retErr = errors.New("must set password")
		return
	}

	opensearch := &opensearchv1.OpenSearchCluster{}
	if err := r.client.Get(
		r.ctx,
		r.spec.OpensearchClusterRef.ObjectKeyFromRef(),
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

	// Handle finalizer
	if r.multiclusterUser != nil {
		if r.multiclusterUser.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.multiclusterUser, meta.OpensearchFinalizer) {
			retErr = r.deleteOpensearchObjects(opensearch)
			return
		}
	}
	if r.loggingMCU != nil {
		if r.loggingMCU.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.loggingMCU, meta.OpensearchFinalizer) {
			retErr = r.deleteOpensearchObjects(opensearch)
			return
		}
	}

	retErr = r.reconcileOpensearchObjects(opensearch)

	return
}

func convertSpec(spec v1beta2.MulticlusterUserSpec) loggingv1beta1.MulticlusterUserSpec {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	retSpec := loggingv1beta1.MulticlusterUserSpec{}
	err = json.Unmarshal(data, &retSpec)
	if err != nil {
		panic(err)
	}
	return retSpec
}
