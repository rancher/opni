package kubernetes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/agentmanifest"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gatewayName = "opni-gateway"
	defaultRepo = "rancher/opni"
)

type AgentPackage string

const (
	agentPackageAgent  AgentPackage = "agent"
	agentPackageClient AgentPackage = "client"
)

type kubernetesResolveImageDriver struct {
	k8sClient client.Client
	namespace string
}

type kubernetesResolveImageDriverOptions struct {
	config *rest.Config
}

type kubernetesResolveImageDriverOption func(*kubernetesResolveImageDriverOptions)

func WithRestConfig(config *rest.Config) kubernetesResolveImageDriverOption {
	return func(o *kubernetesResolveImageDriverOptions) {
		o.config = config
	}
}

func (o *kubernetesResolveImageDriverOptions) apply(opts ...kubernetesResolveImageDriverOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func NewKubernetesResolveImageDriver(
	namespace string,
	opts ...kubernetesResolveImageDriverOption,
) (agentmanifest.ResolveAgentManfestDriver, error) {
	options := kubernetesResolveImageDriverOptions{}
	options.apply(opts...)

	if options.config == nil {
		var err error
		options.config, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	}

	if namespace == "" {
		envNamespace := os.Getenv("POD_NAMESPACE")
		namespace = envNamespace
	}

	k8sClient, err := client.New(options.config, client.Options{
		Scheme: apis.NewScheme(),
	})
	if err != nil {
		return nil, err
	}

	return &kubernetesResolveImageDriver{
		k8sClient: k8sClient,
		namespace: namespace,
	}, nil
}

func (d *kubernetesResolveImageDriver) GetPath(ctx context.Context, _ string) string {
	image := d.getOpniImage(ctx)
	return strings.Split(image, "@")[0]
}

func (d *kubernetesResolveImageDriver) GetDigest(ctx context.Context, packageName string) string {
	switch AgentPackage(packageName) {
	case agentPackageAgent:
		return getMinimalDigest()
	case agentPackageClient:
		image := d.getOpniImage(ctx)
		return strings.Split(image, "@")[1]
	default:
		return ""
	}
}

func (d *kubernetesResolveImageDriver) GetScheme() string {
	return "oci"
}

func init() {
	agentmanifest.RegisterAgentManifestBuilder(
		v1beta1.AgentManifestResolverTypeKubernetes,
		func(args ...any) (agentmanifest.ResolveAgentManfestDriver, error) {
			namespace := args[0].(string)

			var opts []kubernetesResolveImageDriverOption
			for _, arg := range args[1:] {
				switch v := arg.(type) {
				case *rest.Config:
					opts = append(opts, WithRestConfig(v))
				default:
					return nil, fmt.Errorf("unexpected argument: %v", arg)
				}
			}
			return NewKubernetesResolveImageDriver(namespace, opts...)
		},
	)
}
