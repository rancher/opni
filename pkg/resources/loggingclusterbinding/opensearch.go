package loggingclusterbinding

import (
	"errors"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/opensearch"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) reconcileOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
	user := &v1beta2.MulticlusterUser{}
	err := r.client.Get(r.ctx, r.loggingClusterBinding.Spec.MulticlusterUser.ObjectKeyFromRef(), user)
	if err != nil {
		return err
	}

	loggingCluster := &v1beta2.LoggingCluster{}
	if r.loggingClusterBinding.Spec.LoggingCluster.ID != "" {
		list := &v1beta2.LoggingClusterList{}
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

	username, password, err := helpers.UsernameAndPassword(r.client, r.ctx, cluster)
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
