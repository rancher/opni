package loggingclusterbinding

import (
	"github.com/banzaicloud/operator-tools/pkg/reconciler"
	"github.com/rancher/opni/apis/v2beta1"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util/opensearch"
	opensearchapiext "github.com/rancher/opni/pkg/util/opensearch/types"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	opensearchv1 "opensearch.opster.io/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) reconcileOpensearchObjects(cluster *opensearchv1.OpenSearchCluster) error {
	var clustersToReconcile []v2beta1.LoggingCluster

	if len(r.loggingClusterBinding.Spec.DowmstreamClusters) > 0 {
		clusters, err := r.getClusterListFromIDs()
		if err != nil {
			return err
		}
		clustersToReconcile = clusters
	} else if r.loggingClusterBinding.Spec.DownstreamLabelSelector != nil {
		selector := labels.SelectorFromSet(labels.Set(r.loggingClusterBinding.Spec.DownstreamLabelSelector.MatchLabels))
		for _, expression := range r.loggingClusterBinding.Spec.DownstreamLabelSelector.MatchExpressions {
			req, err := labels.NewRequirement(expression.Key, selection.Operator(expression.Operator), expression.Values)
			if err != nil {
				return err
			}
			selector.Add(*req)
		}

		clusterList := &v2beta1.LoggingClusterList{}
		if err := r.client.List(
			r.ctx,
			clusterList,
			client.InNamespace(r.loggingClusterBinding.Namespace),
			client.MatchingLabelsSelector{Selector: selector},
		); err != nil {
			return err
		}
		clustersToReconcile = clusterList.Items
	}

	osPassword, err := r.fetchOpensearchAdminPassword(cluster)
	if err != nil {
		return err
	}
	result := reconciler.CombinedResult{}

	osReconciler := opensearch.NewReconciler(
		r.ctx,
		cluster.Namespace,
		osPassword,
		cluster.Spec.General.ServiceName,
		"todo", // TODO fix dashboards name
	)

	user := opensearchapiext.UserSpec{
		UserName: r.loggingClusterBinding.Spec.Username,
		Password: r.loggingClusterBinding.Spec.Password,
	}

	err = osReconciler.MaybeCreateUser(user)
	if err != nil {
		return err
	}

	for _, cluster := range clustersToReconcile {
		result.CombineErr(osReconciler.MaybeUpdateRolesMapping(cluster.Name, r.loggingClusterBinding.Spec.Username))
	}

	return result.Err
}

// TODO When operator supports alternative passwords fix this up
func (r *Reconciler) fetchOpensearchAdminPassword(cluster *opensearchv1.OpenSearchCluster) (string, error) {
	return "admin", nil
}

func (r *Reconciler) getClusterListFromIDs() ([]v2beta1.LoggingCluster, error) {
	var clusterList []v2beta1.LoggingCluster

	for _, id := range r.loggingClusterBinding.Spec.DowmstreamClusters {
		objectList := &v2beta1.LoggingClusterList{}
		if err := r.client.List(
			r.ctx,
			objectList,
			client.InNamespace(r.loggingClusterBinding.Namespace),
			client.MatchingLabels{resources.OpniClusterID: id},
		); err != nil {
			return clusterList, err
		}
		if len(objectList.Items) == 1 {
			clusterList = append(clusterList, objectList.Items[0])
		}
	}
	return clusterList, nil
}
