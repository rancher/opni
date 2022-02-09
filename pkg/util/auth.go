package util

import (
	"context"
	"math/rand"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GenerateRandomString(length int) []byte {
	rand.Seed(time.Now().UnixNano())
	chars := []byte("BCDFGHJKLMNPQRSTVWXZ" +
		"bcdfghjklmnpqrstvwxz" +
		"0123456789")
	b := make([]byte, length)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))]
	}
	return b
}

func CreateOrUpdate(ctx context.Context, c client.Client, obj client.Object, gvk ...schema.GroupVersionKind) error {
	err := c.Create(ctx, obj)
	if k8serrors.IsAlreadyExists(err) {
		// use unstructured object to update the existing one.
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
		if len(gvk) != 0 {
			u.SetGroupVersionKind(gvk[0])
		}

		err := c.Get(ctx, client.ObjectKeyFromObject(obj), u)
		if err != nil {
			return err
		}

		obj.SetResourceVersion(u.GetResourceVersion())

		err = c.Update(ctx, obj)
		if err != nil {
			return err
		}
	}
	return nil
}
