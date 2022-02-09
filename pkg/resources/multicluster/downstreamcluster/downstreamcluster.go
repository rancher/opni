package downstreamcluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources/multicluster"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client            client.Client
	downstreamCluster *v2beta1.DownstreamCluster
	ctx               context.Context
}

func NewReconciler(
	ctx context.Context,
	downstreamCluster *v2beta1.DownstreamCluster,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:            c,
		downstreamCluster: downstreamCluster,
		ctx:               ctx,
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
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.downstreamCluster), r.downstreamCluster); err != nil {
				return err
			}
			r.downstreamCluster.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.downstreamCluster.Status.IndexUserState = v2beta1.IndexUserStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.downstreamCluster.Status.IndexUserState = v2beta1.IndexUserStatePending
				}
			} else if len(r.downstreamCluster.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.downstreamCluster.Status.IndexUserState = v2beta1.IndexUserStateCreated
			}
			return r.client.Status().Update(r.ctx, r.downstreamCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.downstreamCluster.Spec.LoggingClusterRef == nil {
		retErr = errors.New("logging cluster not provided")
		return
	}

	loggingCluster := &v2beta1.LoggingCluster{}
	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.downstreamCluster.Spec.LoggingClusterRef.Name,
		Namespace: r.downstreamCluster.Namespace,
	}, loggingCluster)
	if retErr != nil {
		return
	}

	retErr = r.maybeGenerateIndexUserSecret()
	if retErr != nil {
		return
	}

	opensearchCluster, retErr := multicluster.FetchOpensearchCluster(r.ctx, r.client, loggingCluster)
	if retErr != nil {
		return
	}

	retResult, retErr = r.ReconcileOpensearchUsers(opensearchCluster)

	return
}

func (r *Reconciler) maybeGenerateIndexUserSecret() error {
	if r.downstreamCluster.Status.IndexUserSecretRef != nil {
		return nil
	}

	name := strings.ToLower(fmt.Sprintf("%s-index-%s", r.downstreamCluster.Name, util.GenerateRandomString(4)))
	password := util.GenerateRandomString(16)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.downstreamCluster.Namespace,
		},
		Data: map[string][]byte{
			"password": password,
		},
	}

	err := ctrl.SetControllerReference(r.downstreamCluster, secret, r.client.Scheme())
	if err != nil {
		return err
	}

	err = r.client.Create(r.ctx, secret)
	if err != nil {
		return err
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.downstreamCluster), r.downstreamCluster); err != nil {
			return err
		}
		r.downstreamCluster.Status.IndexUserSecretRef = &corev1.LocalObjectReference{
			Name: secret.Name,
		}
		return r.client.Status().Update(r.ctx, r.downstreamCluster)
	})

	return err
}
