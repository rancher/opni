package opnicluster

import (
	"context"
	"fmt"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic"
	util "github.com/rancher/opni/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)

	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := util.LoadResult(retResult, retErr)
		if op.ShouldRequeue() {
			if retErr != nil {
				// If an error occurred, the state should be set to error
				r.opniCluster.Status.State = v1beta1.OpniClusterStateError
			} else {
				// If no error occurred, but we need to requeue, the state should be
				// set to working
				r.opniCluster.Status.State = v1beta1.OpniClusterStateWorking
			}
		} else if len(r.opniCluster.Status.Conditions) == 0 {
			// If we are not requeueing and there are no conditions, the state should
			// be set to ready
			r.opniCluster.Status.State = v1beta1.OpniClusterStateReady
		}
		if err := r.client.Status().Update(r.ctx, r.opniCluster); err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	r.opniCluster.Status.Conditions = []string{}

	opniResources := []resources.Resource{
		r.inferenceDeployment,
		r.drainDeployment,
		r.payloadReceiverDeployment,
		r.preprocessingDeployment,
	}

	allResources := []resources.Resource{}
	pretrained, err := r.pretrainedModels()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		r.opniCluster.Status.Conditions =
			append(r.opniCluster.Status.Conditions, err.Error())
		lg.Error(err, "Error when reconciling pretrained models, will retry.")
		// Keep going, we can reconcile the rest of the deployments and come back
		// to this later.
	}
	nats, err := r.nats()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		r.opniCluster.Status.Conditions =
			append(r.opniCluster.Status.Conditions, err.Error())
		lg.Error(err, "Error when reconciling nats, will retry.")
		// Keep going.
	}
	es, err := elastic.NewReconciler(r.client, r.opniCluster).ElasticResources()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		r.opniCluster.Status.Conditions =
			append(r.opniCluster.Status.Conditions, err.Error())
		lg.Error(err, "Error when reconciling elastic, will retry.")
		// Keep going.
	}

	// Order is important
	allResources = append(allResources, nats...)
	allResources = append(allResources, opniResources...)
	allResources = append(allResources, pretrained...)
	allResources = append(allResources, es...)

	for _, factory := range allResources {
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
			retResult = result
		}
	}
	// Update the Nats Replica Status once we have successfully reconciled the opniCluster
	if r.opniCluster.Status.NatsReplicas != *r.getReplicas() {
		r.opniCluster.Status.NatsReplicas = *r.getReplicas()
		if err := r.client.Status().Update(r.ctx, r.opniCluster); err != nil {
			return nil, err
		}
	}
	return
}

func RegisterWatches(builder *builder.Builder) *builder.Builder {
	return builder
}
