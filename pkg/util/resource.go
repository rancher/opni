package util

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CreateOrUpdate(ctx context.Context, c client.Client, obj client.Object) error {
	err := c.Create(ctx, obj)
	if k8serrors.IsAlreadyExists(err) {
		err := c.Update(ctx, obj)
		if err != nil {
			return err
		}
	}
	return nil
}
