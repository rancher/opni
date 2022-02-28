package loggingcluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
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
					r.loggingCluster.Status.State = v2beta1.LoggingClusterStateError
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

	opensearchCluster := &opensearchv1.OpenSearchCluster{}
	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.loggingCluster.Spec.OpensearchClusterRef.Name,
		Namespace: r.loggingCluster.Spec.OpensearchClusterRef.Namespace,
	}, opensearchCluster)
	if retErr != nil {
		return
	}

	switch r.loggingCluster.Status.State {
	case "":
		_, ok := r.loggingCluster.Labels[resources.OpniBootstrapToken]
		if !ok {
			return retResult, errors.New("logging cluster missing bootstrap token")
		}
		retErr = r.maybeGenerateIndexUserSecret()
		if retErr != nil {
			return
		}

		retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
				return err
			}
			r.loggingCluster.Status.State = v2beta1.LoggingClusterStateCreated
			r.loggingCluster.Status.IndexUserState = v2beta1.IndexUserStatePending
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
				r.loggingCluster.Status.State = v2beta1.LoggingClusterStateRegistered
				r.loggingCluster.Status.IndexUserState = v2beta1.IndexUserStateCreated
				return r.client.Status().Update(r.ctx, r.loggingCluster)
			})
		}
	}

	return
}

func (r *Reconciler) maybeGenerateIndexUserSecret() error {
	if r.loggingCluster.Status.IndexUserSecretRef != nil {
		return nil
	}

	name := strings.ToLower(fmt.Sprintf("%s-index-%s", r.loggingCluster.Name, util.GenerateRandomString(4)))
	password := util.GenerateRandomString(16)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.loggingCluster.Namespace,
		},
		Data: map[string][]byte{
			"password": password,
		},
	}

	err := ctrl.SetControllerReference(r.loggingCluster, secret, r.client.Scheme())
	if err != nil {
		return err
	}

	err = r.client.Create(r.ctx, secret)
	if err != nil {
		return err
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
			return err
		}
		r.loggingCluster.Status.IndexUserSecretRef = &corev1.LocalObjectReference{
			Name: secret.Name,
		}
		return r.client.Status().Update(r.ctx, r.loggingCluster)
	})

	return err
}
