package kubernetes

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/rancher/opni/apis"
	corev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/imageresolver"
	"github.com/rancher/opni/pkg/versions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	gatewayName = "opni-gateway"
	defaultRepo = "rancher/opni"
)

type kubernetesResolveImageDriver struct {
	k8sClient client.Client
	namespace string
	tag       string
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
	tag string,
	opts ...kubernetesResolveImageDriverOption,
) (imageresolver.ResolveImageDriver, error) {
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
		tag:       tag,
	}, nil
}

func (d *kubernetesResolveImageDriver) GetImageRepository(ctx context.Context) string {
	gateway := &corev1beta1.Gateway{}
	err := d.k8sClient.Get(ctx, client.ObjectKey{
		Namespace: d.namespace,
		Name:      gatewayName,
	}, gateway)
	if err != nil {
		return defaultRepo
	}

	if gateway.Status.Image != "" {
		return strings.Split(gateway.Status.Image, "@")[0]
	}

	return defaultRepo
}

func (d *kubernetesResolveImageDriver) GetImageTag(_ context.Context) string {
	if d.tag != "" {
		return d.tag
	}

	return versions.Version
}

func (d *kubernetesResolveImageDriver) UseMinimalImage() bool {
	return d.tag == ""
}

func init() {
	imageresolver.RegisterImageResolverBuilder(
		v1beta1.ImageResolverTypeKubernetes,
		func(args ...any) (imageresolver.ResolveImageDriver, error) {
			namespace := args[0].(string)
			tag := args[1].(string)

			var opts []kubernetesResolveImageDriverOption
			for _, arg := range args[2:] {
				switch v := arg.(type) {
				case *rest.Config:
					opts = append(opts, WithRestConfig(v))
				default:
					return nil, fmt.Errorf("unexpected argument: %v", arg)
				}
			}

			return NewKubernetesResolveImageDriver(namespace, tag, opts...)
		},
	)
}
