package capabilities

import corev1 "github.com/rancher/opni/pkg/apis/core/v1"

type TokenCapabilities string

const (
	JoinExistingCluster TokenCapabilities = "join_existing_cluster"
)

func (tc TokenCapabilities) For(ref *corev1.Reference) *corev1.TokenCapability {
	return &corev1.TokenCapability{
		Type:      string(tc),
		Reference: ref,
	}
}
