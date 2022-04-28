package storage

import (
	"context"
	"errors"

	"github.com/rancher/opni/pkg/core"
)

func ApplyRoleBindingTaints(ctx context.Context, store RBACStore, rb *core.RoleBinding) error {
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
