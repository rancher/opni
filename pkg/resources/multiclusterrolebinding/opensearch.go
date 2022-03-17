package multiclusterrolebinding

import (
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
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
	clusterReadRole = osapiext.RoleSpec{
		RoleName: "cluster_read",
		ClusterPermissions: []string{
			"cluster_composite_ops_ro",
		},
		IndexPermissions: []osapiext.IndexPermissionSpec{
			{
				IndexPatterns: []string{
					"logs*",
				},
				DocumentLevelSecurity: clusterTerm,
				AllowedActions: []string{
					"read",
					"search",
				},
			},
		},
	}
)

func (r *Reconciler) ReconcileOpensearchObjects(opensearchCluster *opensearchv1.OpenSearchCluster) (retResult *reconcile.Result, retErr error) {
	username, password, retErr := helpers.UsernameAndPassword(r.client, r.ctx, opensearchCluster)
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

	for _, role := range []osapiext.RoleSpec{
		clusterIndexRole,
		clusterReadRole,
	} {
		retErr = reconciler.MaybeCreateRole(role)
		if retErr != nil {
			return
		}
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

// TODO When operator supports alternative passwords fix this up
func (r *Reconciler) fetchOpensearchAdminPassword(cluster *opensearchv1.OpenSearchCluster) (string, error) {
	return "admin", nil
}
