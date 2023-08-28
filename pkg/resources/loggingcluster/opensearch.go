package loggingcluster

import (
	"errors"
	"fmt"

	"github.com/rancher/opni/pkg/opensearch/certs"
	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearch "github.com/rancher/opni/pkg/opensearch/reconciler"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	ErrorMissingUserSecret = errors.New("user secret not set")

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

func (r *Reconciler) ReconcileOpensearchObjects(
	opensearchCluster *opensearchv1.OpenSearchCluster,
) (retResult *reconcile.Result, retErr error) {
	certMgr := certs.NewCertMgrOpensearchCertManager(
		r.ctx,
		certs.WithNamespace(opensearchCluster.Namespace),
		certs.WithCluster(opensearchCluster.Name),
	)

	reconciler, retErr := opensearch.NewReconciler(
		r.ctx,
		opensearch.ReconcilerConfig{
			CertReader:            certMgr,
			OpensearchServiceName: opensearchCluster.Spec.General.ServiceName,
		},
	)
	if retErr != nil {
		return
	}

	retResult, retErr = r.reconcileOpensearchUsers(reconciler)
	if retErr != nil || retResult != nil {
		return
	}

	retResult, retErr = r.reconcileFriendlyName(reconciler)
	if retErr != nil || retResult != nil {
		return
	}

	return
}

func (r *Reconciler) reconcileOpensearchUsers(reconciler *opensearch.Reconciler) (retResult *reconcile.Result, retErr error) {
	clusterReadRole.RoleName = r.loggingCluster.Name
	clusterReadRole.IndexPermissions[0].DocumentLevelSecurity = fmt.Sprintf(
		`{"term":{"cluster_id": "%s"}}`,
		r.loggingCluster.Labels[resources.OpniClusterID],
	)

	retErr = reconciler.MaybeCreateRole(clusterReadRole)

	return
}

func (r *Reconciler) reconcileFriendlyName(reconciler *opensearch.Reconciler) (retResult *reconcile.Result, retErr error) {
	retErr = reconciler.UpsertClusterMetadata(
		r.loggingCluster.Labels[resources.OpniClusterID],
		r.loggingCluster.Spec.FriendlyName,
		resources.ClusterMetadataIndexName,
	)

	return
}

func (r *Reconciler) deleteOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
	// If the opensearch cluster exists delete the role and user
	if cluster != nil {
		certMgr := certs.NewCertMgrOpensearchCertManager(
			r.ctx,
			certs.WithNamespace(cluster.Namespace),
			certs.WithCluster(cluster.Name),
		)

		osReconciler, err := opensearch.NewReconciler(
			r.ctx,
			opensearch.ReconcilerConfig{
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

		err = osReconciler.DeleteClusterMetadata(
			r.loggingCluster.Labels[resources.OpniClusterID],
			resources.ClusterMetadataIndexName,
		)
		if err != nil {
			return err
		}

		err = osReconciler.DeleteNeuralSearchModel()
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
