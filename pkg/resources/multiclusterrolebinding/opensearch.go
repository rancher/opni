package multiclusterrolebinding

import (
	"fmt"
	"time"

	"github.com/rancher/opni/pkg/opensearch/certs"
	opensearchtypes "github.com/rancher/opni/pkg/opensearch/opensearch/types"
	opensearch "github.com/rancher/opni/pkg/opensearch/reconciler"
	"github.com/rancher/opni/pkg/resources"
	opensearchv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *Reconciler) ReconcileOpensearchObjects(opensearchCluster *opensearchv1.OpenSearchCluster) (retResult *reconcile.Result, retErr error) {
	username, password, retErr := helpers.UsernameAndPassword(r.ctx, r.client, opensearchCluster)
	if retErr != nil {
		return
	}

	certMgr := certs.NewCertMgrOpensearchCertManager(
		r.ctx,
		certs.WithNamespace(opensearchCluster.Namespace),
		certs.WithCluster(opensearchCluster.Name),
	)

	//Generate admin user cert to use
	retErr = certMgr.GenerateClientCert(username)
	if retErr != nil {
		return
	}

	//Generate indexing user for preprocessor to use
	retErr = certMgr.GenerateClientCert(resources.InternalIndexingUser)
	if retErr != nil {
		return
	}

	reconciler, retErr := opensearch.NewReconciler(
		r.ctx,
		opensearch.ReconcilerConfig{

			CertReader:            certMgr,
			OpensearchServiceName: opensearchCluster.Spec.General.ServiceName,
			DashboardsServiceName: fmt.Sprintf("%s-dashboards", opensearchCluster.Spec.General.ServiceName),
		},
		opensearch.WithDashboardsUsername(username),
		opensearch.WithDashboardsPassword(password),
	)
	if retErr != nil {
		return
	}

	// Need to explicitly bind the admin role for cert auth
	retErr = reconciler.MaybeUpdateRolesMapping("all_access", username)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeCreateRole(clusterIndexRole)
	if retErr != nil {
		return
	}

	// bind the indexing user to the index role
	retErr = reconciler.MaybeUpdateRolesMapping(clusterIndexRole.RoleName, resources.InternalIndexingUser)
	if retErr != nil {
		return
	}

	isms := []opensearchtypes.ISMPolicySpec{
		r.logISMPolicy(),
		r.traceISMPolicy(),
	}

	for _, ism := range isms {
		retErr = reconciler.ReconcileISM(ism)
		if retErr != nil {
			return
		}
	}

	retErr = reconciler.MaybeCreateIngestPipeline(preProcessingPipelineName, preprocessingPipeline)
	if retErr != nil {
		return
	}

	templates := []opensearchtypes.IndexTemplateSpec{
		OpniLogTemplate,
		OpniSpanTemplate,
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

	retErr = reconciler.UpdateDefaultIngestPipelineForIndex(
		fmt.Sprintf("%s*", LogIndexPrefix),
		preProcessingPipelineName,
	)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeBootstrapIndex(LogIndexPrefix, LogIndexAlias, OldLogIndexPrefixes)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeBootstrapIndex(SpanIndexPrefix, SpanIndexAlias, OldSpanIndexPrefixes)
	if retErr != nil {
		return
	}

	mappings := map[string]opensearchtypes.TemplateMappingsSpec{
		"mappings": opniServiceMapTemplate.Template.Mappings,
	}
	retErr = reconciler.MaybeCreateIndex(serviceMapIndexName, mappings)
	if retErr != nil {
		return
	}

	retErr = reconciler.MaybeCreateIndex(resources.ClusterMetadataIndexName, clusterMetadataIndexSettings)
	if retErr != nil {
		return
	}

	if opensearchCluster.Spec.Dashboards.Enable {
		retErr = reconciler.ImportKibanaObjects(kibanaDashboardVersionIndex, kibanaDashboardVersionDocID, kibanaDashboardVersion, kibanaObjects)
	}

	return
}
