package gateway

import (
	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) rbac() ([]resources.Resource, error) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.gw.Namespace,
			Labels:    resources.NewGatewayLabels(),
		},
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring-crd",
			Namespace: r.gw.Namespace,
			Labels:    resources.NewGatewayLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{
					"opni.io",
				},
				Resources: []string{
					"bootstraptokens",
					"clusters",
					"keyrings",
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
			Namespace: r.gw.Namespace,
			Labels:    resources.NewGatewayLabels(),
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
				Namespace: r.gw.Namespace,
			},
		},
	}
	ctrl.SetControllerReference(r.gw, serviceAccount, r.client.Scheme())
	ctrl.SetControllerReference(r.gw, role, r.client.Scheme())
	ctrl.SetControllerReference(r.gw, roleBinding, r.client.Scheme())
	return []resources.Resource{
		resources.Present(serviceAccount),
		resources.Present(role),
		resources.Present(roleBinding),
	}, nil
}
