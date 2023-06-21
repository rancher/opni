package opniopensearch

import (
	"context"
	"time"

	"github.com/cisco-open/operator-tools/pkg/reconciler"
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
	ReconcilerOptions
	client   client.Client
	instance *loggingv1beta1.OpniOpensearch
	ctx      context.Context
}

type ReconcilerOptions struct {
	certMgr         certs.OpensearchCertReconcile
	resourceOptions []reconciler.ResourceReconcilerOption
}

type ReconcilerOption func(*ReconcilerOptions)

func (o *ReconcilerOptions) apply(opts ...ReconcilerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithCertManager(certMgr certs.OpensearchCertReconcile) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.certMgr = certMgr
	}
}

func WithResourceOptions(opts ...reconciler.ResourceReconcilerOption) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.resourceOptions = opts
	}
}

func NewReconciler(
	ctx context.Context,
	instance *loggingv1beta1.OpniOpensearch,
	c client.Client,
	opts ...ReconcilerOption,
) *Reconciler {
	options := ReconcilerOptions{}
	options.apply(opts...)

	if options.certMgr == nil {
		certMgr := certs.NewCertMgrOpensearchCertManager(
			ctx,
			certs.WithNamespace(instance.Namespace),
			certs.WithCluster(instance.Name),
		)
		options.certMgr = certMgr.(certs.OpensearchCertReconcile)
	}

	return &Reconciler{
		ReconcilerOptions: options,
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(options.resourceOptions, reconciler.WithLog(log.FromContext(ctx)))...),
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

	err = r.certMgr.GenerateRootCACert()
	if err != nil {
		return nil, err
	}
	err = r.certMgr.GenerateTransportCA()
	if err != nil {
		return nil, err
	}
	err = r.certMgr.GenerateHTTPCA()
	if err != nil {
		return nil, err
	}
	err = r.certMgr.GenerateAdminClientCert()
	if err != nil {
		return nil, err
	}

	result.Combine(r.ReconcileResource(
		r.buildOpensearchCluster(natsSecret, r.certMgr),
		reconciler.StatePresent,
	))
	result.Combine(r.ReconcileResource(r.buildMulticlusterRoleBinding(), reconciler.StatePresent))
	if r.instance.Spec.NatsRef != nil {
		result.Combine(r.ReconcileResource(r.buildConfigMap(), reconciler.StatePresent))
	}
	result.Combine(r.ReconcileResource(r.buildOTELPreprocessor(), reconciler.StateCreated))
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
