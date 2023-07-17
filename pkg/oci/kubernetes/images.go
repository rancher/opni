package kubernetes

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	minimalImageRefEnv = "OPNI_MINIMAL_IMAGE_REF"
	gatewayName        = "opni-gateway"
)

var retryBackoff = wait.Backoff{
	Steps:    4,
	Duration: 500 * time.Millisecond,
	Factor:   2.0,
	Jitter:   0.1,
}

func (d *kubernetesResolveImageDriver) getOpniImageString(ctx context.Context) (string, error) {
	gateway := &corev1beta1.Gateway{}
	retryFunc := func() error {
		err := d.k8sClient.Get(ctx, client.ObjectKey{
			Namespace: d.namespace,
			Name:      gatewayName,
		}, gateway)
		if err != nil {
			return err
		}

		if gateway.Status.Image != "" {
			return nil
		}
		return fmt.Errorf("%w: image is empty", ErrImageNotFound)
	}

	err := retry.OnError(retryBackoff, retriableError, retryFunc)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrImageNotFound, err)
	}

	return gateway.Status.Image, nil
}

func getMinimalImageString() (string, error) {
	envRef := os.Getenv(minimalImageRefEnv)
	if envRef != "" {
		return envRef, nil
	}

	return "", fmt.Errorf("could not find minimal digest in env %s", minimalImageRefEnv)
}
