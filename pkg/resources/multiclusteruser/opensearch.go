package multiclusteruser

import (
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
)

func (r *Reconciler) reconcileOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
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

	osUser := osapiext.UserSpec{
		UserName: r.multiclusterUser.Name,
		Password: r.multiclusterUser.Spec.Password,
		BackendRoles: []string{
			"kibanauser",
		},
	}

	return osReconciler.MaybeCreateUser(osUser)
}
