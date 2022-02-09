package loggingcluster

import (
	"fmt"

	opensearchv1beta1 "github.com/rancher/opni-opensearch-operator/api/v1beta1"
	"github.com/rancher/opni/pkg/resources/multicluster"
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
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

func (r *Reconciler) ReconcileOpensearchObjects(opensearchCluster *opensearchv1beta1.OpensearchCluster) (retResult *reconcile.Result, retErr error) {
	password, retErr := multicluster.FetchOpensearchAdminPassword(r.ctx, r.client, opensearchCluster)
	if retErr != nil {
		return
	}

	reconciler := opensearch.NewReconciler(r.ctx, opensearchCluster.Namespace, password, fmt.Sprintf("%s-os", opensearchCluster.Name))

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
