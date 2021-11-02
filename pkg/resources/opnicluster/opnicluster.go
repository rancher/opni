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
	"github.com/rancher/opni/pkg/resources/opnicluster/thanos"
	"github.com/rancher/opni/pkg/util"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	ErrOpensearchUpgradeFailed = errors.New("opensearch upgrade failed")
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

type resourceSet struct {
	Name    string
	Factory func() ([]resources.Resource, error)
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

	// Reconciler order of operations:
	// 1. Nats
	// 2. S3
	// 3. Elasticsearch
	// 4. Thanos
	// 5. Opni Services
	// 6. Pretrained Models
	//
	// Nats, S3, Elasticsearch, and Thanos reconcilers will add fields to the
	// opniCluster status object which are used by other reconcilers.

	resourceSets := []resourceSet{
		{
			Name:    "Nats",
			Factory: r.nats,
		},
		{
			Name:    "S3",
			Factory: r.s3,
		},
		{
			Name:    "Elasticsearch",
			Factory: elastic.NewReconciler(r.ctx, r.client, r.opniCluster).ElasticResources,
		},
		{
			Name:    "Thanos",
			Factory: thanos.NewReconciler(r.ctx, r.client, r.opniCluster).ThanosResources,
		},
		{
			Name:    "Opni Services",
			Factory: r.opniServices,
		},
		{
			Name:    "Pretrained Models",
			Factory: r.pretrainedModels,
		},
	}

	for i, resourceSet := range resourceSets {
		lg.Info(fmt.Sprintf("Reconciling %s [%d/%d]", resourceSet.Name, i+1, len(resourceSets)))
		resources, err := resourceSet.Factory()
		if err != nil {
			return nil, errors.WrapWithDetails(err, "failed to create resources")
		}
		for _, resource := range resources {
			o, state, err := resource()
			if err != nil {
				return nil, errors.WrapWithDetails(err, "failed to create resource",
					"resource", o.GetObjectKind().GroupVersionKind())
			}
			result, err := r.ReconcileResource(o, state)
			if err != nil {
				return nil, errors.WrapWithDetails(err, "failed to reconcile resource",
					"resource", o.GetObjectKind().GroupVersionKind())
			}
			requeue := util.LoadResult(result, err)
			if requeue.ShouldRequeue() {
				return result, err
			}
		}
	}

	// All resources reconciled successfully, run status checks

	// Check the status of the opensearch data statefulset and update status if it's ready
	osData := &appsv1.StatefulSet{}
	err := r.client.Get(r.ctx, types.NamespacedName{
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
		lg.Error(ErrOpensearchUpgradeFailed, "insufficient data node persistence")
		return
	}

	if (r.opniCluster.Spec.Elastic.Workloads.Master.Replicas == nil ||
		*r.opniCluster.Spec.Elastic.Workloads.Master.Replicas == int32(1)) &&
		(r.opniCluster.Spec.Elastic.Persistence == nil ||
			!r.opniCluster.Spec.Elastic.Persistence.Enabled) {
		lg.Error(ErrOpensearchUpgradeFailed, "insufficient master node persistence")
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
