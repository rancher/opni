package storage

import (
	"context"
	"errors"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func ApplyRoleBindingTaints(ctx context.Context, store RBACStore, rb *corev1.RoleBinding) error {
	rb.Taints = nil
	if _, err := store.GetRole(ctx, rb.RoleReference()); err != nil {
		if errors.Is(err, ErrNotFound) {
			rb.Taints = append(rb.Taints, "role not found")
		} else {
			return err
		}
	}
	if len(rb.Subjects) == 0 {
		rb.Taints = append(rb.Taints, "no subjects")
	}
	return nil
}
