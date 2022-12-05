package loggingclusterbinding

import (
	"errors"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/pkg/util/opensearch"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
	user := &loggingv1beta1.MulticlusterUser{}
	err := r.client.Get(r.ctx, r.loggingClusterBinding.Spec.MulticlusterUser.ObjectKeyFromRef(), user)
	if err != nil {
		return err
	}

	loggingCluster := &corev1beta1.LoggingCluster{}
	if r.loggingClusterBinding.Spec.LoggingCluster.ID != "" {
		list := &corev1beta1.LoggingClusterList{}
		err = r.client.List(
			r.ctx,
			list,
			client.InNamespace(r.loggingClusterBinding.Namespace),
			client.MatchingLabels{resources.OpniClusterID: r.loggingClusterBinding.Spec.LoggingCluster.ID},
		)
		if err != nil {
			return err
		}
		if len(list.Items) != 1 {
			return errors.New("invalid number of logging clusters returned")
		}
		loggingCluster = &list.Items[0]
	} else {
		if r.loggingClusterBinding.Spec.LoggingCluster.LoggingClusterObject == nil {
			return errors.New("either id or logging cluster object must be specified")
		}
		err = r.client.Get(
			r.ctx,
			r.loggingClusterBinding.Spec.LoggingCluster.LoggingClusterObject.ObjectKeyFromRef(),
			loggingCluster,
		)
		if err != nil {
			return err
		}
	}

	// Verify all objects are referring to the same cluster
	if (user.Spec.OpensearchClusterRef == nil || loggingCluster.Spec.OpensearchClusterRef == nil) ||
		(user.Spec.OpensearchClusterRef.Name != cluster.Name || loggingCluster.Spec.OpensearchClusterRef.Name != cluster.Name) ||
		(user.Spec.OpensearchClusterRef.Namespace != cluster.Namespace || loggingCluster.Spec.OpensearchClusterRef.Namespace != cluster.Namespace) {
		return errors.New("opensearch cluster refs must match")
	}

	// Update the status with the referenced objects to allow cleanup
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingClusterBinding), r.loggingClusterBinding); err != nil {
			return err
		}
		r.loggingClusterBinding.Status.Username = user.Name
		r.loggingClusterBinding.Status.Rolename = loggingCluster.Name
		return r.client.Status().Update(r.ctx, r.loggingClusterBinding)
	})
	if err != nil {
		return err
	}

	username, password, err := helpers.UsernameAndPassword(r.ctx, r.client, cluster)
	if err != nil {
		return err
	}

	osReconciler := opensearch.NewReconciler(
		r.ctx,
		cluster.Namespace,
		username,
		password,
		cluster.Spec.General.ServiceName,
		"todo", // TODO fix dashboards name
	)

	return osReconciler.MaybeUpdateRolesMapping(loggingCluster.Name, user.Name)
}

func (r *Reconciler) deleteOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
	username, password, err := helpers.UsernameAndPassword(r.ctx, r.client, cluster)
	if err != nil {
		return err
	}

	osReconciler := opensearch.NewReconciler(
		r.ctx,
		cluster.Namespace,
		username,
		password,
		cluster.Spec.General.ServiceName,
		"todo", // TODO fix dashboards name
	)

	err = osReconciler.MaybeRemoveRolesMapping(r.loggingClusterBinding.Status.Rolename, r.loggingClusterBinding.Status.Username)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingClusterBinding), r.loggingClusterBinding); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.loggingClusterBinding, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.loggingClusterBinding)
	})
}
