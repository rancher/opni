package loggingcluster

import (
	"errors"
	"fmt"
	"time"

	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
	lg := log.FromContext(r.ctx)

	// We should have a user secret when we get to here.  If not requeue the reconciliation loop
	if r.loggingCluster.Status.IndexUserSecretRef == nil {
		lg.V(1).Error(ErrorMissingUserSecret, "indexUserSecret not set, reqeueing")
		retResult = &reconcile.Result{
			RequeueAfter: time.Second * 5,
		}
		return
	}

	indexUser.Attributes = map[string]string{
		"cluster": r.loggingCluster.Labels[v2beta1.IDLabel],
	}

	clusterReadRole.RoleName = r.loggingCluster.Name
	clusterReadRole.IndexPermissions[0].DocumentLevelSecurity = fmt.Sprintf(`{"term":{"cluster_id.keyword": "%s"}}`, r.loggingCluster.Labels[v2beta1.IDLabel])

	secret := &corev1.Secret{}

	retErr = r.client.Get(r.ctx, types.NamespacedName{
		Name:      r.loggingCluster.Status.IndexUserSecretRef.Name,
		Namespace: r.loggingCluster.Namespace,
	}, secret)
	if retErr != nil {
		return
	}

	indexUser.UserName = secret.Name
	indexUser.Password = string(secret.Data["password"])

	osPassword, retErr := r.fetchOpensearchAdminPassword(opensearchCluster)
	if retErr != nil {
		return
	}

	reconciler := opensearch.NewReconciler(r.ctx, opensearchCluster.Namespace, osPassword, fmt.Sprintf("%s-os", opensearchCluster.Name))

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
