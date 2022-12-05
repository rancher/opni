package opniopensearch

import (
	"context"
	"time"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	internalUsername = "internalopni"
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
	natsSecret, requeue, err := r.fetchNatsAuthSecretName()
	if err != nil {
		return nil, err
	}
	if requeue {
		return &reconcile.Result{
			Requeue:      true,
			RequeueAfter: 10 * time.Second,
		}, nil
	}

	result := reconciler.CombinedResult{}

	if !r.instance.Status.PasswordGenerated {
		objects, err := r.generatePasswordObjects()
		if err != nil {
			return nil, err
		}

		for _, object := range objects {
			result.Combine(r.ReconcileResource(object, reconciler.StatePresent))
		}
		if result.Err != nil || !result.Result.IsZero() {
			return &result.Result, result.Err
		}
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.instance), r.instance); err != nil {
				return err
			}

			r.instance.Status.PasswordGenerated = true
			return r.client.Status().Update(r.ctx, r.instance)
		})
		if err != nil {
			return nil, err
		}
	}

	certMgr := certs.NewCertMgrOpensearchCertManager(
		r.ctx,
		certs.WithNamespace(r.instance.Namespace),
		certs.WithCluster(r.instance.Name),
	)
	err = certMgr.GenerateRootCACert()
	if err != nil {
		return nil, err
	}
	err = certMgr.GenerateTransportCA()
	if err != nil {
		return nil, err
	}
	err = certMgr.GenerateHTTPCA()
	if err != nil {
		return nil, err
	}
	err = certMgr.GenerateClientCert(internalUsername)
	if err != nil {
		return nil, err
	}

	result.Combine(r.ReconcileResource(
		r.buildOpensearchCluster(natsSecret, certMgr.(certs.K8sOpensearchCertManager)),
		reconciler.StatePresent,
	))
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
