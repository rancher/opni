package multiclusterrolebinding

import (
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices"
	"github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	clusterTerm = `{"term":{"cluster_id.keyword": "${attr.internal.cluster}"}}`

	clusterIndexRole = osapiext.RoleSpec{
		RoleName: "cluster_index",
		ClusterPermissions: []string{
			"cluster_composite_ops",
			"cluster_monitor",
		},
		IndexPermissions: []osapiext.IndexPermissionSpec{
			{
				IndexPatterns: []string{
					"logs*",
				},
				AllowedActions: []string{
					"index",
					"indices:admin/get",
				},
			},
		},
	}
)

func (r *Reconciler) ReconcileOpensearchObjects(opensearchCluster *opensearchv1.OpenSearchCluster) (retResult *reconcile.Result, retErr error) {
	username, password, retErr := helpers.UsernameAndPassword(r.ctx, r.client, opensearchCluster)
	if retErr != nil {
		return
	}

	reconciler := opensearch.NewReconciler(
		r.ctx,
		opensearchCluster.Namespace,
		username,
		password,
		opensearchCluster.Spec.General.ServiceName,
		"todo", // TODO fix dashboards name
	)

	retErr = reconciler.MaybeCreateRole(clusterIndexRole)
	if retErr != nil {
		return
	}

	retErr = reconciler.ReconcileISM(indices.OpniLogPolicy)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeCreateIndexTemplate(&indices.OpniLogTemplate)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeBootstrapIndex(indices.LogIndexPrefix, indices.LogIndexAlias)

	return
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

	err = osReconciler.MaybeDeleteRole(clusterIndexRole.RoleName)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiClusterRoleBinding), r.multiClusterRoleBinding); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.multiClusterRoleBinding, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.multiClusterRoleBinding)
	})
}
