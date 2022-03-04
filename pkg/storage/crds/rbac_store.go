package crds

import (
	"context"

	"github.com/rancher/opni-monitoring/pkg/core"
	"github.com/rancher/opni-monitoring/pkg/sdk/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *CRDStore) CreateRole(ctx context.Context, role *core.Role) error {
	return c.client.Create(ctx, &v1beta1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      role.Id,
			Namespace: c.namespace,
		},
		Spec: role,
	})
}

func (c *CRDStore) DeleteRole(ctx context.Context, ref *core.Reference) error {
	return c.client.Delete(ctx, &v1beta1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
}

func (c *CRDStore) GetRole(ctx context.Context, ref *core.Reference) (*core.Role, error) {
	role := &v1beta1.Role{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      ref.Id,
		Namespace: c.namespace,
	}, role)
	if err != nil {
		return nil, err
	}
	return role.Spec, nil
}

func (c *CRDStore) CreateRoleBinding(ctx context.Context, rb *core.RoleBinding) error {
	return c.client.Create(ctx, &v1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rb.Id,
			Namespace: c.namespace,
		},
		Spec: rb,
	})
}

func (c *CRDStore) DeleteRoleBinding(ctx context.Context, ref *core.Reference) error {
	return c.client.Delete(ctx, &v1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
}

func (c *CRDStore) GetRoleBinding(ctx context.Context, ref *core.Reference) (*core.RoleBinding, error) {
	rb := &v1beta1.RoleBinding{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      ref.Id,
		Namespace: c.namespace,
	}, rb)
	if err != nil {
		return nil, err
	}
	return rb.Spec, nil
}

func (c *CRDStore) ListRoles(ctx context.Context) (*core.RoleList, error) {
	list := &v1beta1.RoleList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	roles := &core.RoleList{
		Items: make([]*core.Role, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		roles.Items = append(roles.Items, item.Spec)
	}
	return roles, nil
}

func (c *CRDStore) ListRoleBindings(ctx context.Context) (*core.RoleBindingList, error) {
	list := &v1beta1.RoleBindingList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	rb := &core.RoleBindingList{
		Items: make([]*core.RoleBinding, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		rb.Items = append(rb.Items, item.Spec)
	}
	return rb, nil
}
