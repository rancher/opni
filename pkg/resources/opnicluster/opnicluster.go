package opnicluster

import (
	"context"
	"fmt"
	"time"

	"emperror.dev/errors"
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	aiv1beta1 "github.com/rancher/opni/apis/ai/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	prometheusRuleFinalizer = "opni.io/prometheusRule"
)

var (
	ErrOpensearchUpgradeFailed = errors.New("opensearch upgrade failed")
)

type Reconciler struct {
	reconciler.ResourceReconciler
	ctx               context.Context
	client            client.Client
	recorder          record.EventRecorder
	opniCluster       *v1beta2.OpniCluster
	aiOpniCluster     *aiv1beta1.OpniCluster
	instanceName      string
	instanceNamespace string
	spec              v1beta2.OpniClusterSpec
	opensearchCluster *opensearchv1.OpenSearchCluster
}

func NewReconciler(
	ctx context.Context,
	client client.Client,
	recorder record.EventRecorder,
	instance interface{},
	opts ...reconciler.ResourceReconcilerOption,
) (*Reconciler, error) {
	r := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(client,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		ctx:      ctx,
		client:   client,
		recorder: recorder,
	}

	switch cluster := instance.(type) {
	case *v1beta2.OpniCluster:
		r.instanceName = cluster.Name
		r.instanceNamespace = cluster.Namespace
		r.spec = cluster.Spec
		r.opniCluster = cluster
	case *aiv1beta1.OpniCluster:
		r.instanceName = cluster.Name
		r.instanceNamespace = cluster.Namespace
		r.spec = ConvertSpec(cluster.Spec)
		r.aiOpniCluster = cluster
	default:
		return nil, errors.New("invalid opnicluster instance type")
	}

	return r, nil
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the opnicluster
		// is and set it in the state field accordingly.
		op := util.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if r.opniCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
					return err
				}
				r.opniCluster.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.opniCluster.Status.State = v1beta2.OpniClusterStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.opniCluster.Status.State = v1beta2.OpniClusterStateWorking
					}
				} else if len(r.opniCluster.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.opniCluster.Status.State = v1beta2.OpniClusterStateReady
					// Set the opensearch version once it's been created
					if r.opniCluster.Status.OpensearchState.Version == nil {
						r.opniCluster.Status.OpensearchState.Version = &r.spec.Opensearch.Version
					}
				}
				return r.client.Status().Update(r.ctx, r.opniCluster)
			}
			if r.aiOpniCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.aiOpniCluster), r.aiOpniCluster); err != nil {
					return err
				}
				r.aiOpniCluster.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.aiOpniCluster.Status.State = aiv1beta1.OpniClusterStateError
					} else {
						// If no error occurred, but we need to requeue, the state should be
						// set to working
						r.aiOpniCluster.Status.State = aiv1beta1.OpniClusterStateWorking
					}
				} else if len(r.aiOpniCluster.Status.Conditions) == 0 {
					// If we are not requeueing and there are no conditions, the state should
					// be set to ready
					r.aiOpniCluster.Status.State = aiv1beta1.OpniClusterStateReady
				}
				return r.client.Status().Update(r.ctx, r.aiOpniCluster)
			}
			return errors.New("no opnicluster instance to update")
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	// Handle finalizer
	if r.opniCluster != nil {
		if r.opniCluster.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.opniCluster, prometheusRuleFinalizer) {
			retResult, retErr = r.cleanupPrometheusRule()
			if retErr != nil || retResult != nil {
				return
			}
		}
	}
	if r.aiOpniCluster != nil {
		if r.aiOpniCluster.DeletionTimestamp != nil && controllerutil.ContainsFinalizer(r.aiOpniCluster, prometheusRuleFinalizer) {
			retResult, retErr = r.cleanupPrometheusRule()
			if retErr != nil || retResult != nil {
				return
			}
		}
	}

	// GPU learning is currently unsupported but will be added back in soon
	if lo.FromPtrOr(r.spec.Services.GPUController.Enabled, true) {
		lg.Info("gpu learning is currently not supported, but will return in a later release")
		if r.opniCluster != nil {
			r.recorder.Event(r.opniCluster, "Normal", "GPU service not supported", "the GPU service will be available in a later release")
		}
		if r.aiOpniCluster != nil {
			r.recorder.Event(r.aiOpniCluster, "Normal", "GPU service not supported", "the GPU service will be available in a later release")
		}
	}

	if r.spec.Opensearch.ExternalOpensearch != nil {
		r.opensearchCluster = &opensearchv1.OpenSearchCluster{}
		err := r.client.Get(r.ctx, r.spec.Opensearch.ExternalOpensearch.ObjectKeyFromRef(), r.opensearchCluster)
		if err != nil {
			if !k8serrors.IsNotFound(err) {
				return nil, err
			}
			lg.V(1).Error(err, "external opensearch object does not exist")
		}
	}

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

	var es []resources.Resource
	if r.opensearchCluster == nil && r.opniCluster != nil {
		es, err = elastic.NewReconciler(r.ctx, r.client, r.opniCluster).OpensearchResources()
		if err != nil {
			retErr = errors.Combine(retErr, err)
			conditions = append(conditions, err.Error())
			lg.Error(err, "Error when reconciling elastic, will retry.")
			return
		}
	} else {
		if r.opensearchCluster == nil {
			lg.Error(err, "external opensearch not found")
			return
		}
		es, err = r.externalOpensearchConfig()
		if err != nil {
			retErr = errors.Combine(retErr, err)
			conditions = append(conditions, err.Error())
			lg.Error(err, "Error when setting external opensearch config")
			return
		}
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
	if r.opensearchCluster == nil {
		osData := &appsv1.StatefulSet{}
		err = r.client.Get(r.ctx, types.NamespacedName{
			Name:      elastic.OpniDataWorkload,
			Namespace: r.instanceNamespace,
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
	} else {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opensearchCluster), r.opensearchCluster); err != nil {
				return err
			}
			if r.opniCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
					return err
				}
				r.opniCluster.Status.OpensearchState.Initialized = r.opensearchCluster.Status.Phase == opensearchv1.PhaseRunning
				return r.client.Status().Update(r.ctx, r.opniCluster)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Update the Nats Replica Status once we have successfully reconciled the opniCluster
	if r.opniCluster != nil {
		err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
				return err
			}
			replicas := lo.FromPtrOr(r.spec.Nats.Replicas, 3)
			if r.opniCluster.Status.NatsReplicas != replicas {
				r.opniCluster.Status.NatsReplicas = replicas
			}
			return r.client.Status().Update(r.ctx, r.opniCluster)
		})

		if err != nil {
			return nil, err
		}
	}

	return
}

func (r *Reconciler) ReconcileOpensearchUpgrade() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)

	// IF we're using an external Opensearch do nothing
	if r.spec.Opensearch.ExternalOpensearch != nil {
		return
	}

	if r.opniCluster.Status.OpensearchState.Version == nil || *r.opniCluster.Status.OpensearchState.Version == r.spec.Opensearch.Version {
		return
	}

	// If no persistence and only one data replica we can't safely upgrade so log an error and return
	if (r.spec.Opensearch.Workloads.Data.Replicas == nil ||
		*r.spec.Opensearch.Workloads.Data.Replicas == int32(1)) &&
		(r.spec.Opensearch.Persistence == nil ||
			!r.spec.Opensearch.Persistence.Enabled) {
		lg.Error(ErrOpensearchUpgradeFailed, "insufficient data node persistence")
		return
	}

	if (r.spec.Opensearch.Workloads.Master.Replicas == nil ||
		*r.spec.Opensearch.Workloads.Master.Replicas == int32(1)) &&
		(r.spec.Opensearch.Persistence == nil ||
			!r.spec.Opensearch.Persistence.Enabled) {
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
		r.opniCluster.Status.OpensearchState.Version = &r.spec.Opensearch.Version
		return r.client.Status().Update(r.ctx, r.opniCluster)
	})

	return
}

func (r *Reconciler) cleanupPrometheusRule() (retResult *reconcile.Result, retErr error) {
	var namespace string
	if r.opniCluster != nil {
		namespace = r.opniCluster.Status.PrometheusRuleNamespace
	}
	if r.aiOpniCluster != nil {
		namespace = r.aiOpniCluster.Status.PrometheusRuleNamespace
	}
	if namespace == "" {
		retErr = errors.New("prometheusRule namespace is unknown")
		return
	}
	prometheusRule := &monitoringv1.PrometheusRule{}
	err := r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf("%s-%s", v1beta2.MetricsService.ServiceName(), r.generateSHAID()),
		Namespace: namespace,
	}, prometheusRule)
	if k8serrors.IsNotFound(err) {
		retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if r.opniCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.opniCluster), r.opniCluster); err != nil {
					return err
				}
				controllerutil.RemoveFinalizer(r.opniCluster, prometheusRuleFinalizer)
				return r.client.Update(r.ctx, r.opniCluster)
			}
			if r.aiOpniCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.aiOpniCluster), r.aiOpniCluster); err != nil {
					return err
				}
				controllerutil.RemoveFinalizer(r.aiOpniCluster, prometheusRuleFinalizer)
				return r.client.Update(r.ctx, r.aiOpniCluster)
			}
			return nil
		})
		return
	} else if err != nil {
		retErr = err
		return
	}
	retErr = r.client.Delete(r.ctx, prometheusRule)
	if retErr != nil {
		return
	}
	return &reconcile.Result{RequeueAfter: time.Second}, nil
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
					r.opniCluster.Status.LogCollectorState = v1beta2.OpniClusterStateError
				} else {
					// If no error occurred, but we need to requeue, the state should be
					// set to working
					r.opniCluster.Status.LogCollectorState = v1beta2.OpniClusterStateWorking
				}
			} else if len(r.opniCluster.Status.Conditions) == 0 {
				// If we are not requeueing and there are no conditions, the state should
				// be set to ready
				r.opniCluster.Status.LogCollectorState = v1beta2.OpniClusterStateReady
			}
			return r.client.Status().Update(r.ctx, r.opniCluster)
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	desiredState := deploymentState(r.spec.DeployLogCollector)
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

func (r *Reconciler) setOwner(obj client.Object) error {
	if r.opniCluster != nil {
		err := ctrl.SetControllerReference(r.opniCluster, obj, r.client.Scheme())
		if err != nil {
			return err
		}
	}
	if r.aiOpniCluster != nil {
		err := ctrl.SetControllerReference(r.aiOpniCluster, obj, r.client.Scheme())
		if err != nil {
			return err
		}
	}
	return nil
}

func RegisterWatches(builder *builder.Builder) *builder.Builder {
	return builder
}
