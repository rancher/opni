package opnicluster

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type DesiredObject struct {
	Object runtime.Object
	State  reconciler.DesiredState
}

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx         context.Context
	client      client.Client
	opniCluster *v1beta1.OpniCluster
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	opniCluster *v1beta1.OpniCluster,
	opts ...func(*reconciler.ReconcilerOpts),
) *Reconciler {
	return &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		ctx:         ctx,
		client:      client,
		opniCluster: opniCluster,
	}
}

func (r *Reconciler) Reconcile() (*reconcile.Result, error) {
	pretrained := r.pretrainedModels()
	for _, factory := range append([]resources.Resource{
		r.inferenceDeployment,
		r.drainDeployment,
		r.payloadReceiverDeployment,
		r.preprocessingDeployment,
	}, pretrained...) {
		o, state, err := factory()
		if err != nil {
			return nil, errors.WrapIf(err, "failed to create object")
		}
		if o == nil {
			panic(fmt.Sprintf("reconciler %#v created a nil object", factory))
		}
		result, err := r.ReconcileResource(o, state)
		if err != nil {
			return nil, errors.WrapWithDetails(err, "failed to reconcile resource",
				"resource", o.GetObjectKind().GroupVersionKind())
		}
		if result != nil {
			return result, nil
		}
	}
	return nil, nil
}

func RegisterWatches(builder *builder.Builder) *builder.Builder {
	return builder
}
