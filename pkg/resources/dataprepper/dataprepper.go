package dataprepper

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/cisco-open/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ReconcilerOptions
	client      client.Client
	dataPrepper *loggingv1beta1.DataPrepper
	ctx         context.Context
}

type ReconcilerOptions struct {
	forceInsecure   bool
	urlOverride     string
	resourceOptions []reconciler.ResourceReconcilerOption
}

type ReconcilerOption func(*ReconcilerOptions)

func (o *ReconcilerOptions) apply(opts ...ReconcilerOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithForceInsecure() ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.forceInsecure = true
	}
}

func WithURLOverride(urlOverride string) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.urlOverride = urlOverride
	}
}

func WithResourceOptions(opts ...reconciler.ResourceReconcilerOption) ReconcilerOption {
	return func(o *ReconcilerOptions) {
		o.resourceOptions = opts
	}
}

func NewReconciler(
	ctx context.Context,
	instance *loggingv1beta1.DataPrepper,
	c client.Client,
	opts ...ReconcilerOption,
) *Reconciler {
	options := ReconcilerOptions{}
	options.apply(opts...)

	return &Reconciler{
		ReconcilerOptions: options,
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(options.resourceOptions, reconciler.WithLog(log.FromContext(ctx)))...),
		client:      c,
		dataPrepper: instance,
		ctx:         ctx,
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
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.dataPrepper), r.dataPrepper); err != nil {
				return err
			}
			r.dataPrepper.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.dataPrepper.Status.State = loggingv1beta1.DataprepperStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.dataPrepper.Status.State = loggingv1beta1.DataPrepperStatePending
				}
			} else if len(r.dataPrepper.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.dataPrepper.Status.State = loggingv1beta1.DataPrepperStateReady
			}
			return r.client.Status().Update(r.ctx, r.dataPrepper)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.dataPrepper.Spec.PasswordFrom == nil {
		retErr = errors.New("missing password secret configuration")
		return
	}

	if r.dataPrepper.Spec.Opensearch == nil {
		retErr = errors.New("missing opensearch configuration")
	}

	var resourceList []resources.Resource

	config, data := r.config()

	resourceList = append(resourceList, config)
	resourceList = append(resourceList, r.service())
	resourceList = append(resourceList, r.deployment(data))

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
