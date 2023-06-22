package preprocessor

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/cisco-open/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	ReconcilerOptions
	reconciler.ResourceReconciler
	client       client.Client
	preprocessor *loggingv1beta1.Preprocessor
	ctx          context.Context
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
	instance *loggingv1beta1.Preprocessor,
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
		client:       c,
		preprocessor: instance,
		ctx:          ctx,
	}
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := k8sutil.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.preprocessor), r.preprocessor); err != nil {
				return err
			}
			r.preprocessor.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.preprocessor.Status.State = loggingv1beta1.PreprocessorStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.preprocessor.Status.State = loggingv1beta1.PreprocessorStatePending
				}
			} else if len(r.preprocessor.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.preprocessor.Status.State = loggingv1beta1.PreprocessorStateReady
			}
			return r.client.Status().Update(r.ctx, r.preprocessor)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.preprocessor.Spec.OpensearchCluster == nil {
		return nil, errors.New("missing opensearch reference")
	}

	var resourceList []resources.Resource

	config, configHash := r.configMap()
	resourceList = append(resourceList, config)
	resourceList = append(resourceList, r.caChain())
	resourceList = append(resourceList, r.deployment(configHash))
	resourceList = append(resourceList, r.service())

	for _, factory := range resourceList {
		o, state, err := factory()
		if err != nil {
			retErr = errors.WrapIf(err, "failed to create object")
			return
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			retErr = errors.WrapWithDetails(err, "failed to reconcile resource",
				"resource", o.GetObjectKind().GroupVersionKind())
			return
		}
		if result != nil {
			retResult = result
		}
	}

	return
}
