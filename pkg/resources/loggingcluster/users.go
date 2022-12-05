package loggingcluster

import (
	"errors"
	"fmt"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearch "github.com/rancher/opni/pkg/opensearch/reconciler"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/meta"
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

	indexUser = opensearchtypes.UserSpec{}

	clusterReadRole = opensearchtypes.RoleSpec{
		RoleName: "cluster_read",
		ClusterPermissions: []string{
			"cluster_composite_ops_ro",
		},
		IndexPermissions: []opensearchtypes.IndexPermissionSpec{
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
		"cluster": r.loggingCluster.Labels[corev1beta1.IDLabel],
	}

	clusterReadRole.RoleName = r.loggingCluster.Name
	clusterReadRole.IndexPermissions[0].DocumentLevelSecurity = fmt.Sprintf(
		`{"term":{"cluster_id": "%s"}}`,
		r.loggingCluster.Labels[resources.OpniClusterID],
	)

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

	username, _, retErr := helpers.UsernameAndPassword(r.ctx, r.client, opensearchCluster)
	if retErr != nil {
		return
	}

	certMgr := certs.NewCertMgrOpensearchCertManager(
		r.ctx,
		certs.WithNamespace(opensearchCluster.Namespace),
		certs.WithCluster(opensearchCluster.Name),
	)

	reconciler, retErr := opensearch.NewReconciler(
		r.ctx,
		opensearch.ReconcilerConfig{
			Namespace:             opensearchCluster.Namespace,
			Username:              username,
			CertReader:            certMgr,
			OpensearchServiceName: opensearchCluster.Spec.General.ServiceName,
		},
	)
	if retErr != nil {
		return
	}

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
	// If the opensearch cluster exists delete the role and user
	if cluster != nil {
		username, _, err := helpers.UsernameAndPassword(r.ctx, r.client, cluster)
		if err != nil {
			return err
		}

		certMgr := certs.NewCertMgrOpensearchCertManager(
			r.ctx,
			certs.WithNamespace(cluster.Namespace),
			certs.WithCluster(cluster.Name),
		)

		osReconciler, err := opensearch.NewReconciler(
			r.ctx,
			opensearch.ReconcilerConfig{
				Namespace:             cluster.Namespace,
				Username:              username,
				CertReader:            certMgr,
				OpensearchServiceName: cluster.Spec.General.ServiceName,
			},
		)
		if err != nil {
			return err
		}

		err = osReconciler.MaybeDeleteRole(cluster.Name)
		if err != nil {
			return err
		}

		err = osReconciler.MaybeDeleteUser(r.loggingCluster.Spec.IndexUserSecret.Name)
		if err != nil {
			return err
		}
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.loggingCluster), r.loggingCluster); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.loggingCluster, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.loggingCluster)
	})
}
