package common

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CreateDefaultNamespace creates the default opni namespace if it does not
// already exist.
func CreateDefaultNamespace(ctx context.Context) error {
	return createNS(ctx, DefaultOpniNamespace)
}

// CreateDefaultNamespace creates the default opni demo namespace if it does not
// already exist.
func CreateDefaultDemoNamespace(ctx context.Context) error {
	return createNS(ctx, DefaultOpniDemoNamespace)
}

func createNS(ctx context.Context, ns string) error {
	if err := K8sClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ns,
		},
	}); errors.IsAlreadyExists(err) {
		Log.Info(err)
	} else if err != nil {
		return err
	}
	return nil
}
