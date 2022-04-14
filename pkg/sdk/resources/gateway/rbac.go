package gateway

import (
	"github.com/rancher/opni/pkg/sdk/resources"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) rbac() ([]resources.Resource, error) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.gateway.Namespace,
			Labels:    resources.Labels(),
		},
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring-crd",
			Namespace: r.gateway.Namespace,
			Labels:    resources.Labels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{
					"monitoring.opni.io",
				},
				Resources: []string{
					"bootstraptokens",
					"clusters",
					"rolebindings",
					"roles",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"patch",
					"delete",
				},
			},
			{
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"secrets",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"patch",
					"delete",
				},
			},
		},
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring-crd",
			Namespace: r.gateway.Namespace,
			Labels:    resources.Labels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "opni-monitoring-crd",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount.Name,
				Namespace: r.gateway.Namespace,
			},
		},
	}
	ctrl.SetControllerReference(r.gateway, serviceAccount, r.client.Scheme())
	ctrl.SetControllerReference(r.gateway, role, r.client.Scheme())
	ctrl.SetControllerReference(r.gateway, roleBinding, r.client.Scheme())
	return []resources.Resource{
		resources.Present(serviceAccount),
		resources.Present(role),
		resources.Present(roleBinding),
	}, nil
}
