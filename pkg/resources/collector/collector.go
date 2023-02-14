package collector

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client    client.Client
	collector *corev1beta1.Collector
	ctx       context.Context
}

func NewReconciler(
	ctx context.Context,
	instance *corev1beta1.Collector,
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client:    c,
		collector: instance,
		ctx:       ctx,
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
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.collector), r.collector); err != nil {
				return err
			}
			r.collector.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.collector.Status.State = corev1beta1.CollectorStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.collector.Status.State = corev1beta1.CollectorStatePending
				}
			} else if len(r.collector.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.collector.Status.State = corev1beta1.CollectorStateReady
			}
			return r.client.Status().Update(r.ctx, r.collector)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	var resourceList []resources.Resource

	config, configHash := r.agentConfigMap()
	resourceList = append(resourceList, config)
	resourceList = append(resourceList, r.daemonSet(configHash))

	config, configHash = r.aggregatorConfigMap()
	resourceList = append(resourceList, config)
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
