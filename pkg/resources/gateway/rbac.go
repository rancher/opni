package gateway

import (
	"fmt"

	"github.com/rancher/opni/pkg/resources"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Reconciler) rbac() ([]resources.Resource, error) {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring",
			Namespace: r.namespace,
			Labels:    resources.NewGatewayLabels(),
		},
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring-crd",
			Namespace: r.namespace,
			Labels:    resources.NewGatewayLabels(),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{
					"opni.io",
				},
				Resources: []string{
					"bootstraptokens",
					"loggingclusters",
					"monitoringclusters",
					"multiclusterrolebindings",
					"clusters",
					"keyrings",
					"rolebindings",
					"roles",
					"opniopensearches",
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
					"logging.opni.io",
					"monitoring.opni.io",
					"core.opni.io",
					"ai.opni.io",
				},
				Resources: []string{
					"*",
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
					"opensearch.opster.io",
				},
				Resources: []string{
					"opensearchclusters",
				},
				Verbs: []string{
					"get",
					"list",
					"watch",
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
			{
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"configmaps",
				},
				Verbs: []string{
					"get",
					"list",
					"update",
					"patch",
					"watch",
				},
			},
		},
	}

	// TODO: This will leak.  Add a finalizer to fix it up or come up with alternative
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   fmt.Sprintf("opni-monitoring-ns-%s", r.name),
			Labels: resources.NewGatewayLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "opni-monitoring-ns",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount.Name,
				Namespace: r.namespace,
			},
		},
	}

	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "opni-monitoring-crd",
			Namespace: r.namespace,
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
				Namespace: r.namespace,
			},
		},
	}

	r.setOwner(serviceAccount)
	r.setOwner(role)
	r.setOwner(roleBinding)
	return []resources.Resource{
		resources.Present(serviceAccount),
		resources.Present(role),
		resources.Present(roleBinding),
		resources.Present(clusterRoleBinding),
	}, nil
}
