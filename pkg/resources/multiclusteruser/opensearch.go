package multiclusteruser

import (
	opensearchv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	osapiext "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearch "github.com/rancher/opni/pkg/opensearch/reconciler"
	"github.com/rancher/opni/pkg/util/meta"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcileOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
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

	osUser := osapiext.UserSpec{
		UserName: r.multiclusterUser.Name,
		Password: r.multiclusterUser.Spec.Password,
		BackendRoles: []string{
			"kibanauser",
		},
	}

	return osReconciler.MaybeCreateUser(osUser)
}

func (r *Reconciler) deleteOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
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

		err = osReconciler.MaybeDeleteUser(r.multiclusterUser.Name)
		if err != nil {
			return err
		}
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiclusterUser), r.multiclusterUser); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.multiclusterUser, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.multiclusterUser)
	})
}
