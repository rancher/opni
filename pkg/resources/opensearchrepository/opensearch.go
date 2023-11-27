package opensearchrepository

import (
	"errors"

	opensearchv1 "github.com/Opster/opensearch-k8s-operator/opensearch-operator/api/v1"
	"github.com/rancher/opni/pkg/opensearch/certs"
	osapi "github.com/rancher/opni/pkg/opensearch/opensearch/types"
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

	settings := osapi.RepositoryRequest{}
	switch {
	case r.respository.Spec.Settings.S3 != nil:
		settings.Type = osapi.RepositoryTypeS3
		settings.Settings.S3Settings = &osapi.S3Settings{
			Bucket: r.respository.Spec.Settings.S3.Bucket,
			Path:   r.respository.Spec.Settings.S3.Folder,
		}
	case r.respository.Spec.Settings.FileSystem != nil:
		settings.Type = osapi.RepositoryTypeFileSystem
		settings.Settings.FileSystemSettings = &osapi.FileSystemSettings{
			Location: r.respository.Spec.Settings.FileSystem.Location,
		}
	default:
		return errors.New("invalid repository settings")
	}

	return osReconciler.MaybeUpdateRepository(r.respository.Name, settings)
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

		err = osReconciler.MaybeDeleteRepository(r.respository.Name)
		if err != nil {
			return err
		}
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.respository), r.respository); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.respository, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.respository)
	})
}
