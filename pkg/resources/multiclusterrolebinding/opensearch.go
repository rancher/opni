package multiclusterrolebinding

import (
	"time"

	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/resources/opnicluster/elastic/indices"
	"github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/pkg/util/opensearch"
	osapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"k8s.io/client-go/util/retry"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) ReconcileOpensearchObjects(opensearchCluster *opensearchv1.OpenSearchCluster) (retResult *reconcile.Result, retErr error) {
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

	retErr = reconciler.MaybeCreateRole(clusterIndexRole)
	if retErr != nil {
		return
	}

	isms := []osapiext.ISMPolicySpec{
		r.logISMPolicy(),
	}
	if features.FeatureList.FeatureIsEnabled("tracing") {
		isms = append(isms, r.traceISMPolicy())
	}

	for _, ism := range isms {
		retErr = reconciler.ReconcileISM(ism)
		if retErr != nil {
			return
		}
	}

	templates := []osapiext.IndexTemplateSpec{
		indices.OpniLogTemplate,
	}
	if features.FeatureList.FeatureIsEnabled("tracing") {
		templates = append(templates, opniSpanTemplate)
	}

	for _, template := range templates {
		retErr = reconciler.MaybeCreateIndexTemplate(template)
		if retErr != nil {
			return
		}
		exists, err := reconciler.TemplateExists(template.TemplateName)
		if err != nil {
			retErr = err
			return
		}

		if !exists {
			retResult = &reconcile.Result{
				Requeue:      true,
				RequeueAfter: 5 * time.Second,
			}
		}
	}

	retErr = reconciler.MaybeBootstrapIndex(indices.LogIndexPrefix, indices.LogIndexAlias, indices.OldIndexPrefixes)
	if retErr != nil {
		return
	}

	if features.FeatureList.FeatureIsEnabled("tracing") {
		retErr = reconciler.MaybeBootstrapIndex(spanIndexPrefix, spanIndexAlias, oldTracingIndexPrefixes)
		if retErr != nil {
			return
		}

		mappings := map[string]osapiext.TemplateMappingsSpec{
			"mappings": opniServiceMapTemplate.Template.Mappings,
		}
		retErr = reconciler.MaybeCreateIndex(serviceMapIndexName, mappings)
		if retErr != nil {
			return
		}
	}

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

	err = osReconciler.MaybeDeleteRole(clusterIndexRole.RoleName)
	if err != nil {
		return err
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := r.client.Get(r.ctx, client.ObjectKeyFromObject(r.multiClusterRoleBinding), r.multiClusterRoleBinding); err != nil {
			return err
		}
		controllerutil.RemoveFinalizer(r.multiClusterRoleBinding, meta.OpensearchFinalizer)
		return r.client.Update(r.ctx, r.multiClusterRoleBinding)
	})
}
