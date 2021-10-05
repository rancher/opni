package util

import (
	"context"
	"math/rand"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateRandomPassword() []byte {
	rand.Seed(time.Now().UnixNano())
	chars := []byte("BCDFGHJKLMNPQRSTVWXZ" +
		"bcdfghjklmnpqrstvwxz" +
		"0123456789")
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return b
}

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
