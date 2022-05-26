package capabilities

import corev1 "github.com/rancher/opni/pkg/apis/core/v1"

func Cluster(name string) *corev1.ClusterCapability {
	return &corev1.ClusterCapability{
		Name: name,
	}
}
