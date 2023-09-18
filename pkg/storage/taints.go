package storage

import (
	"context"
	"errors"
	"fmt"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

func ApplyRoleBindingTaints(ctx context.Context, store RBACStore, rb *corev1.RoleBinding) error {
	rb.Taints = nil
	for _, roleId := range rb.GetRoleIds() {
		if _, err := store.GetRole(ctx, &corev1.Reference{Id: roleId}); err != nil {
			if errors.Is(err, ErrNotFound) {
				rb.Taints = append(rb.Taints, fmt.Sprintf("role %s not found", roleId))
			} else {
				return err
			}
		}
	}
	if rb.Subject == "" {
		rb.Taints = append(rb.Taints, "no subject")
	}
	return nil
}
