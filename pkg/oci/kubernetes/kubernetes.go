package kubernetes

import (
	"context"
	"fmt"
	"os"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/oci"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
) (oci.Fetcher, error) {
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

func (d *kubernetesResolveImageDriver) GetImage(ctx context.Context, imageType oci.ImageType) (oci.Image, error) {
	var image oci.Image
	switch imageType {
	case oci.ImageTypeOpni:
		image = d.getOpniImage(ctx)
	case oci.ImageTypeMinimal:
		image = d.getMinimalImage(ctx)
	default:
		return oci.Image{}, ErrUnsupportedImageType
	}
	if image.Repository == "" {
		return oci.Image{}, ErrImageNotFound
	}
	return image, nil
}

func (d *kubernetesResolveImageDriver) getOpniImage(ctx context.Context) oci.Image {
	return oci.Parse(d.getOpniImageString(ctx))
}

func (d *kubernetesResolveImageDriver) getMinimalImage(ctx context.Context) oci.Image {
	image := d.getOpniImage(ctx)
	minimalDigest := getMinimalDigest()
	if minimalDigest != "" {
		image.Digest = minimalDigest
	}
	return image
}

func init() {
	oci.RegisterFetcherBuilder("kubernetes", func(args ...any) (oci.Fetcher, error) {
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
	})
}
