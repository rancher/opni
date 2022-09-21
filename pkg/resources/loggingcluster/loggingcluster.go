package loggingcluster

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Reconciler struct {
	reconciler.ResourceReconciler
	client             client.Client
	loggingCluster     *v1beta2.LoggingCluster
	coreLoggingCluster *corev1beta1.LoggingCluster
	instanceName       string
	instanceNamespace  string
	labels             map[string]string
	state              string
	spec               corev1beta1.LoggingClusterSpec
	ctx                context.Context
}

func NewReconciler(
	ctx context.Context,
	instance interface{},
	c client.Client,
	opts ...reconciler.ResourceReconcilerOption,
) (*Reconciler, error) {
	r := &Reconciler{
		ResourceReconciler: reconciler.NewReconcilerWith(c,
			append(opts, reconciler.WithLog(log.FromContext(ctx)))...),
		client: c,
		ctx:    ctx,
	}

	switch cluster := instance.(type) {
	case *v1beta2.LoggingCluster:
		r.instanceName = cluster.Name
		r.instanceNamespace = cluster.Namespace
		r.labels = cluster.Labels
		r.state = string(cluster.Status.State)
		r.spec = convertSpec(cluster.Spec)
		r.loggingCluster = cluster
	case *corev1beta1.LoggingCluster:
		r.instanceName = cluster.Name
		r.instanceNamespace = cluster.Namespace
		r.labels = cluster.Labels
		r.state = string(cluster.Status.State)
		r.spec = cluster.Spec
		r.coreLoggingCluster = cluster
	default:
		return nil, errors.New("invalid loggingcluster instance type")
	}

	return r, nil
}

func (r *Reconciler) Reconcile() (retResult *reconcile.Result, retErr error) {
	lg := log.FromContext(r.ctx)
	conditions := []string{}

	defer func() {
		// When the reconciler is done, figure out what the state of the loggingcluster
		// is and set it in the state field accordingly.
		op := k8sutil.LoadResult(retResult, retErr)
		err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if r.loggingCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
					return err
				}
				r.loggingCluster.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.loggingCluster.Status.State = v1beta2.LoggingClusterStateError
					}
				}
				return r.client.Status().Update(r.ctx, r.loggingCluster)
			}
			if r.coreLoggingCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.coreLoggingCluster), r.coreLoggingCluster); err != nil {
					return err
				}
				r.coreLoggingCluster.Status.Conditions = conditions
				if op.ShouldRequeue() {
					if retErr != nil {
						// If an error occurred, the state should be set to error
						r.coreLoggingCluster.Status.State = corev1beta1.LoggingClusterStateError
					}
				}
				return r.client.Status().Update(r.ctx, r.coreLoggingCluster)
			}
			return errors.New("no loggingcluster instance to update")
		})

		if err != nil {
			lg.Error(err, "failed to update status")
		}
	}()

	if r.loggingCluster != nil {
		if r.loggingCluster.Spec.OpensearchClusterRef == nil {
			retErr = errors.New("logging cluster not provided")
			return
		}

		if r.loggingCluster.Spec.IndexUserSecret == nil {
			retErr = errors.New("index user secret not provided")
			return
		}
	}
	if r.coreLoggingCluster != nil {
		if r.coreLoggingCluster.Spec.OpensearchClusterRef == nil {
			retErr = errors.New("logging cluster not provided")
			return
		}

		if r.coreLoggingCluster.Spec.IndexUserSecret == nil {
			retErr = errors.New("index user secret not provided")
			return
		}
	}

	opensearchCluster := &opensearchv1.OpenSearchCluster{}
	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.spec.OpensearchClusterRef.Name,
		Namespace: r.spec.OpensearchClusterRef.Namespace,
	}, opensearchCluster)
	if retErr != nil {
		return
	}

	// Handle finalizer
	if r.loggingCluster != nil &&
		r.loggingCluster.DeletionTimestamp != nil &&
		controllerutil.ContainsFinalizer(r.loggingCluster, meta.OpensearchFinalizer) {
		retErr = r.deleteOpensearchObjects(opensearchCluster)
		return
	}
	if r.coreLoggingCluster != nil &&
		r.coreLoggingCluster.DeletionTimestamp != nil &&
		controllerutil.ContainsFinalizer(r.coreLoggingCluster, meta.OpensearchFinalizer) {
		retErr = r.deleteOpensearchObjects(opensearchCluster)
		return
	}

	switch r.state {
	case "":
		retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if r.loggingCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
					return err
				}
				r.loggingCluster.Status.State = v1beta2.LoggingClusterStateCreated
				r.loggingCluster.Status.IndexUserState = v1beta2.IndexUserStatePending
				return r.client.Status().Update(r.ctx, r.loggingCluster)
			}
			if r.coreLoggingCluster != nil {
				if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.coreLoggingCluster), r.coreLoggingCluster); err != nil {
					return err
				}
				r.coreLoggingCluster.Status.State = corev1beta1.LoggingClusterStateCreated
				r.coreLoggingCluster.Status.IndexUserState = corev1beta1.IndexUserStatePending
				return r.client.Status().Update(r.ctx, r.coreLoggingCluster)
			}
			return errors.New("no loggingcluster instance to update")
		})
		return
	default:
		_, ok := r.labels[resources.OpniClusterID]
		if ok {
			retResult, retErr = r.ReconcileOpensearchUsers(opensearchCluster)
			if retErr != nil {
				return
			}
			retErr = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				if r.loggingCluster != nil {
					if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
						return err
					}
					r.loggingCluster.Status.State = v1beta2.LoggingClusterStateRegistered
					r.loggingCluster.Status.IndexUserState = v1beta2.IndexUserStateCreated
					return r.client.Status().Update(r.ctx, r.loggingCluster)
				}
				if r.coreLoggingCluster != nil {
					if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.coreLoggingCluster), r.coreLoggingCluster); err != nil {
						return err
					}
					r.coreLoggingCluster.Status.State = corev1beta1.LoggingClusterStateRegistered
					r.coreLoggingCluster.Status.IndexUserState = corev1beta1.IndexUserStateCreated
					return r.client.Status().Update(r.ctx, r.coreLoggingCluster)
				}
				return errors.New("no loggingcluster instance to update")
			})
		}
	}

	return
}

func convertSpec(spec v1beta2.LoggingClusterSpec) corev1beta1.LoggingClusterSpec {
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	retSpec := corev1beta1.LoggingClusterSpec{}
	err = json.Unmarshal(data, &retSpec)
	if err != nil {
		panic(err)
	}
	return retSpec
}
