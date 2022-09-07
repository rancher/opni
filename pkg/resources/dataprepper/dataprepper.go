package dataprepper

import (
	"context"
	"encoding/json"
	"fmt"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client             client.Client
	dataPrepper        *v1beta2.DataPrepper
	loggingDataPrepper *loggingv1beta1.DataPrepper
	ctx                context.Context
	instanceName       string
	instanceNamespace  string
	spec               loggingv1beta1.DataPrepperSpec
}

func NewReconciler(
	ctx context.Context,
	instance interface{},
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) (*Reconciler, error) {
	reconciler := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client: c,
		ctx:    ctx,
	}
	switch prepper := instance.(type) {
	case *v1beta2.DataPrepper:
		reconciler.dataPrepper = prepper
		reconciler.instanceName = prepper.Name
		reconciler.instanceNamespace = prepper.Namespace
		reconciler.spec = convertSpec(prepper.Spec)
		return reconciler, nil
	case *loggingv1beta1.DataPrepper:
		reconciler.loggingDataPrepper = prepper
		reconciler.instanceName = prepper.Name
		reconciler.instanceNamespace = prepper.Namespace
		reconciler.spec = prepper.Spec
		return reconciler, nil
	default:
		return nil, errors.New("invalid dataprepper instance type")
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
			if r.dataPrepper != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.dataPrepper), r.dataPrepper); err != nil {
					return err
				}
				r.dataPrepper.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.dataPrepper.Status.State = v1beta2.DataprepperStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.dataPrepper.Status.State = v1beta2.DataPrepperStatePending
					}
				} else if len(r.dataPrepper.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.dataPrepper.Status.State = v1beta2.DataPrepperStateReady
				}
				return r.client.Status().Update(r.ctx, r.dataPrepper)
			}
			if r.loggingDataPrepper != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingDataPrepper), r.loggingDataPrepper); err != nil {
					return err
				}
				r.loggingDataPrepper.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.loggingDataPrepper.Status.State = loggingv1beta1.DataprepperStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.loggingDataPrepper.Status.State = loggingv1beta1.DataPrepperStatePending
					}
				} else if len(r.loggingDataPrepper.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.loggingDataPrepper.Status.State = loggingv1beta1.DataPrepperStateReady
				}
				return r.client.Status().Update(r.ctx, r.loggingDataPrepper)
			}
			return errors.New("no datapreper instance to update")
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.dataPrepper != nil && r.dataPrepper.Spec.PasswordFrom == nil {
		retErr = errors.New("missing password secret configuration")
		return
	}

	if r.loggingDataPrepper != nil && r.loggingDataPrepper.Spec.PasswordFrom == nil {
		retErr = errors.New("missing password secret configuration")
		return
	}

	if r.dataPrepper != nil && r.dataPrepper.Spec.Opensearch == nil {
		retErr = errors.New("missing opensearch configuration")
	}

	if r.loggingDataPrepper != nil && r.loggingDataPrepper.Spec.Opensearch == nil {
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

func convertSpec(spec v1beta2.DataPrepperSpec) loggingv1beta1.DataPrepperSpec {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	retSpec := loggingv1beta1.DataPrepperSpec{}
	err = json.Unmarshal(data, &retSpec)
	if err != nil {
		panic(err)
	}
	return retSpec
}
