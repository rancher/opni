package multicluster

import (
	"context"

	opensearchv1beta1 "github.com/rancher/opni-opensearch-operator/api/v1beta1"
	"github.com/rancher/opni/apis/v2beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	OpensearchClusterNameDefault = "default"
)

func FetchOpensearchCluster(
	ctx context.Context,
	client client.Client,
	loggingCluster *v2beta1.LoggingCluster,
) (opensearchCluster *opensearchv1beta1.OpensearchCluster, retErr error) {
	var clusterName types.NamespacedName
	opensearchCluster = &opensearchv1beta1.OpensearchCluster{}

	if loggingCluster.Spec.OpensearchCluster == nil {
		clusterName = types.NamespacedName{
			Name:      OpensearchClusterNameDefault,
			Namespace: loggingCluster.Namespace,
		}
	} else {
		clusterName = loggingCluster.Spec.OpensearchCluster.ObjectKeyFromRef()
	}

	retErr = client.Get(ctx, clusterName, opensearchCluster)

	return
}

func FetchOpensearchAdminPassword(
	ctx context.Context,
	client client.Client,
	opensearchCluster *opensearchv1beta1.OpensearchCluster,
) (password string, retErr error) {
	passwordSecret := &corev1.Secret{}
	retErr = client.Get(ctx, types.NamespacedName{
		Name:      opensearchCluster.Status.Auth.OpensearchAuthSecretKeyRef.Name,
		Namespace: opensearchCluster.Namespace,
	}, passwordSecret)
	if retErr != nil {
		return
	}

	password = string(passwordSecret.Data[opensearchCluster.Status.Auth.OpensearchAuthSecretKeyRef.Key])
	return
}
