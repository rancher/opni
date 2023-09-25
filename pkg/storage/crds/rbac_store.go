package crds

import (
	"context"

	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

func (c *CRDStore) UpdateRoleBinding(ctx context.Context, ref *corev1.Reference, mutator storage.MutatorFunc[*corev1.RoleBinding]) (*corev1.RoleBinding, error) {
	var rb *corev1.RoleBinding
	err := retry.OnError(defaultBackoff, k8serrors.IsConflict, func() error {
		existing := &monitoringv1beta1.RoleBinding{}
		err := c.client.Get(ctx, client.ObjectKey{
			Name:      ref.Id,
			Namespace: c.namespace,
		}, existing)
		if err != nil {
			return err
		}
		clone := existing.DeepCopy()
		mutator(clone.Spec)
		rb = clone.Spec
		err = c.client.Update(ctx, clone)
		rb.SetResourceVersion(clone.GetObjectMeta().GetResourceVersion())
		return err
	})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return rb, nil
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
	rb.Spec.SetResourceVersion(rb.GetResourceVersion())
	return rb.Spec, nil
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
