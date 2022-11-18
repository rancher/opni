package crds

import (
	"context"

	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (c *CRDStore) CreateRole(ctx context.Context, role *corev1.Role) error {
	err := c.client.Create(ctx, &monitoringv1beta1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      role.Id,
			Namespace: c.namespace,
		},
		Spec: role,
	})
	if k8serrors.IsAlreadyExists(err) {
		return storage.ErrAlreadyExists
	}
	return err
}

func (c *CRDStore) DeleteRole(ctx context.Context, ref *corev1.Reference) error {
	err := c.client.Delete(ctx, &monitoringv1beta1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
	if k8serrors.IsNotFound(err) {
		return storage.ErrNotFound
	}
	return err
}

func (c *CRDStore) GetRole(ctx context.Context, ref *corev1.Reference) (*corev1.Role, error) {
	role := &monitoringv1beta1.Role{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      ref.Id,
		Namespace: c.namespace,
	}, role)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return role.Spec, nil
}

func (c *CRDStore) CreateRoleBinding(ctx context.Context, rb *corev1.RoleBinding) error {
	err := c.client.Create(ctx, &monitoringv1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rb.Id,
			Namespace: c.namespace,
		},
		Spec: rb,
	})
	if k8serrors.IsAlreadyExists(err) {
		return storage.ErrAlreadyExists
	}
	return err
}

func (c *CRDStore) DeleteRoleBinding(ctx context.Context, ref *corev1.Reference) error {
	err := c.client.Delete(ctx, &monitoringv1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Id,
			Namespace: c.namespace,
		},
	})
	if k8serrors.IsNotFound(err) {
		return storage.ErrNotFound
	}
	return err
}

func (c *CRDStore) GetRoleBinding(ctx context.Context, ref *corev1.Reference) (*corev1.RoleBinding, error) {
	rb := &monitoringv1beta1.RoleBinding{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      ref.Id,
		Namespace: c.namespace,
	}, rb)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return rb.Spec, nil
}

func (c *CRDStore) ListRoles(ctx context.Context) (*corev1.RoleList, error) {
	list := &monitoringv1beta1.RoleList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	roles := &corev1.RoleList{
		Items: make([]*corev1.Role, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		roles.Items = append(roles.Items, item.Spec)
	}
	return roles, nil
}

func (c *CRDStore) ListRoleBindings(ctx context.Context) (*corev1.RoleBindingList, error) {
	list := &monitoringv1beta1.RoleBindingList{}
	err := c.client.List(ctx, list, client.InNamespace(c.namespace))
	if err != nil {
		return nil, err
	}
	rb := &corev1.RoleBindingList{
		Items: make([]*corev1.RoleBinding, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		rb.Items = append(rb.Items, item.Spec)
	}
	return rb, nil
}
