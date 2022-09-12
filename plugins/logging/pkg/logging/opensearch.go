package logging

import (
	"context"

	loggingv1beta1 "github.com/rancher/opni/apis/logging/v1beta1"
	"github.com/rancher/opni/pkg/features"
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/plugins/logging/pkg/apis/opensearch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (p *Plugin) GetDetails(ctx context.Context, cluster *opensearch.ClusterReference) (*opensearch.OpensearchDetails, error) {

	// Get the external URL
	binding := &loggingv1beta1.MulticlusterRoleBinding{}
	opnimgmt := &loggingv1beta1.OpniOpensearch{}

	if p.manageFlag.IsEnabled() {
		if err := p.k8sClient.Get(ctx, types.NamespacedName{
			Name:      p.opensearchCluster.Name,
			Namespace: p.storageNamespace,
		}, opnimgmt); err != nil {
			return nil, err
		}
	} else {
		if err := p.k8sClient.Get(ctx, types.NamespacedName{
			Name:      OpensearchBindingName,
			Namespace: p.storageNamespace,
		}, binding); err != nil {
			return nil, err
		}
	}

	labels := map[string]string{
		resources.OpniClusterID: cluster.AuthorizedClusterID,
	}
	secrets := &corev1.SecretList{}
	if err := p.k8sClient.List(ctx, secrets, client.InNamespace(p.storageNamespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	if len(secrets.Items) != 1 {
		return nil, ErrGetDetailsInvalidList(cluster.AuthorizedClusterID)
	}

	return &opensearch.OpensearchDetails{
		Username:       secrets.Items[0].Name,
		Password:       string(secrets.Items[0].Data["password"]),
		ExternalURL:    binding.Spec.OpensearchExternalURL,
		TracingEnabled: features.FeatureList.FeatureIsEnabled("tracing"),
	}, nil
}
