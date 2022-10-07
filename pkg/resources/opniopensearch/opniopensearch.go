package opniopensearch

import (
	"context"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client   client.Client
	instance *loggingv1beta1.OpniOpensearch
	ctx      context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *loggingv1beta1.OpniOpensearch,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:   c,
		ctx:      ctx,
		instance: instance,
	}
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	natsSecret, err := r.fetchNatsAuthSecretName()
	if err != nil {
		return nil, err
	}

	result := reconciler.CombinedResult{}
	result.Combine(r.ReconcileResource(r.buildOpensearchCluster(natsSecret), reconciler.StatePresent))
	result.Combine(r.ReconcileResource(r.buildMulticlusterRoleBinding(), reconciler.StatePresent))
	if r.instance.Spec.NatsRef != nil {
		result.Combine(r.ReconcileResource(r.buildConfigMap(), reconciler.StatePresent))
	}

	if result.Err != nil || !result.Result.IsZero() {
		return &result.Result, result.Err
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.instance), r.instance); err != nil {
			return err
		}

		r.instance.Status.Version = &r.instance.Spec.Version
		r.instance.Status.OpensearchVersion = &r.instance.Spec.OpensearchVersion

		return r.client.Status().Update(r.ctx, r.instance)
	})
	return &result.Result, err
}
