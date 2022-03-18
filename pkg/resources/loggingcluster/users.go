package loggingcluster

import (
	"errors"
	"fmt"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	ErrorMissingUserSecret = errors.New("user secret not set")

	indexUser = osapiext.UserSpec{}

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
				AllowedActions: []string{
					"read",
					"search",
				},
			},
		},
	}
)

func (r *Reconciler) ReconcileOpensearchUsers(opensearchCluster *opensearchv1.OpenSearchCluster) (retResult *reconcile.Result, retErr error) {
	indexUser.Attributes = map[string]string{
		"cluster": r.loggingCluster.Labels[v1beta2.IDLabel],
	}

	clusterReadRole.RoleName = r.loggingCluster.Name
	clusterReadRole.IndexPermissions[0].DocumentLevelSecurity = fmt.Sprintf(`{"term":{"cluster_id.keyword": "%s"}}`, r.loggingCluster.Labels[resources.OpniClusterID])

	secret := &corev1.Secret{}

	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      fmt.Sprintf(r.loggingCluster.Spec.IndexUserSecret.Name),
		Namespace: r.loggingCluster.Namespace,
	}, secret)
	if retErr != nil {
		return
	}

	indexUser.UserName = fmt.Sprintf(r.loggingCluster.Spec.IndexUserSecret.Name)
	indexUser.Password = string(secret.Data["password"])

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

	retErr = reconciler.MaybeCreateRole(clusterReadRole)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeCreateUser(indexUser)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeUpdateRolesMapping("cluster_index", indexUser.UserName)

	return
}

// TODO When operator supports alternative passwords fix this up
func (r *Reconciler) fetchOpensearchAdminPassword(cluster *opensearchv1.OpenSearchCluster) (string, error) {
	return "admin", nil
}
