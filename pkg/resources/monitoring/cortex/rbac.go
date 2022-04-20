package cortex

import (
	"github.com/rancher/opni/pkg/resources"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) serviceAccount() resources.Resource {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cortex",
			Namespace: r.mc.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "cortex",
			},
		},
		AutomountServiceAccountToken: util.Pointer(true),
	}
	return resources.Present(sa)
}
