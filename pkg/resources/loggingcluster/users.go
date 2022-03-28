package loggingcluster

import (
	"errors"
	"fmt"

	"github.com/rancher/opni/apis/v1beta2"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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

	err = osReconciler.MaybeDeleteRole(r.loggingCluster.Name)
	if err != nil {
		return err
	}

	err = osReconciler.MaybeDeleteUser(r.loggingCluster.Spec.IndexUserSecret.Name)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.loggingCluster, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.loggingCluster)
	})
}
