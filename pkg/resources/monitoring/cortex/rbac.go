package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) serviceAccount() resources.Resource {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex",
			Namespace: r.namespace,
			Labels:    cortexAppLabel,
		},
		AutomountServiceAccountToken: lo.ToPtr(true),
	}
	return resources.PresentIff(r.spec.Cortex.Enabled, sa)
}
