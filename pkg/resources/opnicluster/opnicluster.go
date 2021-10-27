package opnicluster

import (
	"context"
	"fmt"
	"time"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := util.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
				return err
			}
			r.opniCluster.Status.Conditions = conditions
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
				// Set the opensearch version once it's been created
				if r.opniCluster.Status.OpensearchState.Version == nil {
					r.opniCluster.Status.OpensearchState.Version = &r.opniCluster.Spec.Elastic.Version
				}
			}
			return r.client.Status().Update(r.ctx, r.opniCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	allResources := []resources.Resource{}
	opniServices, err := r.opniServices()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		return nil, err
	}
	pretrained, err := r.pretrainedModels()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling pretrained models, will retry.")
		// Keep going, we can reconcile the rest of the deployments and come back
		// to this later.
	}
	es, err := elastic.NewReconciler(r.ctx, r.client, r.opniCluster).ElasticResources()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling elastic, will retry.")
		return
	}
	nats, err := r.nats()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling nats, cannot continue.")
		return
	}
	var s3 []resources.Resource
	intS3, err := r.internalS3()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling s3, cannot continue.")
		return
	}
	extS3, err := r.externalS3()
	if err != nil {
		retErr = errors.Combine(retErr, err)
		conditions = append(conditions, err.Error())
		lg.Error(err, "Error when reconciling s3, cannot continue.")
		return
	}
	s3 = append(s3, intS3...)
	s3 = append(s3, extS3...)

	// Order is important here
	// nats, s3, and elasticsearch reconcilers will add fields to the opniCluster status object
	// which are used by other reconcilers.
	allResources = append(allResources, nats...)
	allResources = append(allResources, s3...)
	allResources = append(allResources, es...)
	allResources = append(allResources, opniServices...)
	allResources = append(allResources, pretrained...)

	for _, factory := range allResources {
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

	// Check the status of the opensearch data statefulset and update status if it's ready
	osData := &appsv1.StatefulSet{}
	err = r.client.Get(r.ctx, types.NamespacedName{
		Name:      elastic.OpniDataWorkload,
		Namespace: r.opniCluster.Namespace,
	}, osData)
	if err != nil {
		return nil, err
	}

	if osData.Spec.Replicas != nil && osData.Status.ReadyReplicas == *osData.Spec.Replicas {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
				return err
			}
			r.opniCluster.Status.OpensearchState.Initialized = true
			return r.client.Status().Update(r.ctx, r.opniCluster)
		})
		if err != nil {
			return nil, err
		}
	}

	// Update the Nats Replica Status once we have successfully reconciled the opniCluster
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
			return err
		}
		if r.opniCluster.Status.NatsReplicas != *r.getReplicas() {
			r.opniCluster.Status.NatsReplicas = *r.getReplicas()
		}
		return r.client.Status().Update(r.ctx, r.opniCluster)
	})

	if err != nil {
		return nil, err
	}

	return
}

func (r *Reconciler) ReconcileElasticUpgrade() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	if r.opniCluster.Status.OpensearchState.Version == nil || *r.opniCluster.Status.OpensearchState.Version == r.opniCluster.Spec.Elastic.Version {
		return
	}

	// If no persistence and only one data replica we can't safely upgrade so log an error and return
	if (r.opniCluster.Spec.Elastic.Workloads.Data.Replicas == nil ||
		*r.opniCluster.Spec.Elastic.Workloads.Data.Replicas == int32(1)) &&
		(r.opniCluster.Spec.Elastic.Persistence == nil ||
			!r.opniCluster.Spec.Elastic.Persistence.Enabled) {
		lg.Error(errors.New("insufficient data persistence"), "can't upgrade opensearch")
		return
	}

	if (r.opniCluster.Spec.Elastic.Workloads.Master.Replicas == nil ||
		*r.opniCluster.Spec.Elastic.Workloads.Master.Replicas == int32(1)) &&
		(r.opniCluster.Spec.Elastic.Persistence == nil ||
			!r.opniCluster.Spec.Elastic.Persistence.Enabled) {
		lg.Error(errors.New("insufficient master persistence"), "can't upgrade opensearch")
		return
	}

	es := elastic.NewReconciler(r.ctx, r.client, r.opniCluster)

	// Update data nodes first
	requeue, err := es.UpgradeData()
	if err != nil {
		return retResult, err
	}
	if requeue {
		retResult = &reconcile.Result{
			RequeueAfter: 5 * time.Second,
		}
		return
	}

	retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
			return err
		}
		r.opniCluster.Status.OpensearchState.Version = &r.opniCluster.Spec.Elastic.Version
		return r.client.Status().Update(r.ctx, r.opniCluster)
	})

	return
}

func (r *Reconciler) ReconcileLogCollector() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := util.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
				return err
			}
			r.opniCluster.Status.Conditions = conditions
			if op.ShouldRequeue() {
				if retErr != nil {
					// If an error occurred, the state should be set to error
					r.opniCluster.Status.LogCollectorState = v1beta1.OpniClusterStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.opniCluster.Status.LogCollectorState = v1beta1.OpniClusterStateWorking
				}
			} else if len(r.opniCluster.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.opniCluster.Status.LogCollectorState = v1beta1.OpniClusterStateReady
			}
			return r.client.Status().Update(r.ctx, r.opniCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	desiredState := deploymentState(r.opniCluster.Spec.DeployLogCollector)
	clusterOutput := r.buildClusterOutput()
	clusterFlow := r.buildClusterFlow()

	for _, object := range []client.Object{
		clusterOutput,
		clusterFlow,
	} {
		result, err := r.ReconcileResource(object, desiredState)
		if err != nil {
			retErr = errors.WrapWithDetails(err, "failed to reconcile resource",
				"resource", object.GetObjectKind().GroupVersionKind())
			return
		}
		if result != nil {
			retResult = result
		}
	}

	return
}

func RegisterWatches(builder *builder.Builder) *builder.Builder {
	return builder
}
