package kubernetes

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/versions"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const minimalDigestPath = "/var/lib/opni/minimal-digest"

var retryBackoff = wait.Backoff{
	Steps:    4,
	Duration: 5 * time.Second,
	Factor:   2.0,
	Jitter:   0.1,
}

func getMinimalDigest() string {
	if _, err := os.Stat(minimalDigestPath); err == nil {
		digest, err := os.ReadFile(minimalDigestPath)
		if err != nil {
			return ""
		}
		return string(digest)
	}

	// TODO: remove this once we have solved the problem getting at the digest of the minimal image
	if versions.Version != "unversioned" {
		return fmt.Sprintf("%s-minimal", versions.Version)
	}

	return ""
}

func (d *kubernetesResolveImageDriver) getOpniImage(ctx context.Context) string {
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
		return ErrImageNotFound
	}

	err := retry.OnError(retryBackoff, retriableError, retryFunc)
	if err != nil {
		return ""
	}

	return gateway.Status.Image
}
