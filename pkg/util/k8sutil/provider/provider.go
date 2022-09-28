package provider

import (
	"context"
	"errors"

	"github.com/rancher/kubernetes-provider-detector/providers"
	"k8s.io/client-go/kubernetes"
)

var allProviders = make(map[string]IsProvider)

// IsProvider is the interface all providers need to implement
type IsProvider func(ctx context.Context, k8sClient kubernetes.Interface) (bool, error)

var ErrUnknownProvider = errors.New("unknown provider")

func init() {
	allProviders[providers.AKS] = providers.IsAKS
	allProviders[providers.EKS] = providers.IsEKS
	allProviders[providers.GKE] = providers.IsGKE
	allProviders[providers.K3s] = providers.IsK3s
	allProviders[providers.RKE] = providers.IsRKE
	allProviders[providers.RKE2] = providers.IsRKE2
}

// DetectProvider accepts a k8s interface and checks all registered providers for a match
func DetectProvider(ctx context.Context, k8sClient kubernetes.Interface) (string, error) {
	for name, p := range allProviders {
		// Check the context before calling the provider
		if err := ctx.Err(); err != nil {
			return "", err
		}

		if ok, err := p(ctx, k8sClient); err != nil {
			return "", err
		} else if ok {
			return name, nil
		}
	}
	return "generic", nil
}
