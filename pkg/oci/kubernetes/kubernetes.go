package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/oci"
	"github.com/rancher/opni/pkg/versions"
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

type KubernetesResolveImageDriverOption func(*kubernetesResolveImageDriverOptions)

func WithRestConfig(config *rest.Config) KubernetesResolveImageDriverOption {
	return func(o *kubernetesResolveImageDriverOptions) {
		o.config = config
	}
}

func (o *kubernetesResolveImageDriverOptions) apply(opts ...KubernetesResolveImageDriverOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func NewKubernetesResolveImageDriver(
	namespace string,
	opts ...KubernetesResolveImageDriverOption,
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

func (d *kubernetesResolveImageDriver) GetImage(ctx context.Context, imageType oci.ImageType) (*oci.Image, error) {
	var image *oci.Image
	var err error
	switch imageType {
	case oci.ImageTypeOpni:
		image, err = d.getOpniImage(ctx)
	case oci.ImageTypeMinimal:
		image, err = d.getMinimalImage(ctx)
	default:
		return nil, ErrUnsupportedImageType
	}

	if err != nil {
		return nil, err
	}

	if image.Empty() {
		return nil, ErrImageNotFound
	}
	return image, nil
}

func (d *kubernetesResolveImageDriver) getOpniImage(ctx context.Context) (*oci.Image, error) {
	imageStr, err := d.getOpniImageString(ctx)
	if err != nil {
		return nil, fmt.Errorf("error resolving opni image: %w", err)
	}
	return oci.Parse(imageStr)
}

func (d *kubernetesResolveImageDriver) getMinimalImage(ctx context.Context) (*oci.Image, error) {
	minimalImageStr, minimalImageErr := getMinimalImageString()
	if minimalImageErr == nil {
		// use the minimal image from the environment
		return oci.Parse(minimalImageStr)
	}

	// no minimal image available, try to guess based on the full image
	opniImage, opniImageErr := d.getOpniImage(ctx)
	if opniImageErr == nil {
		// if we have a version, we can append the "-minimal" suffix to the tag
		// to get the tagged minimal image for the same version
		if versions.Version != "unversioned" {
			opniImage.Tag = versions.Version + "-minimal"
			opniImage.Digest = ""
		}
		// no version, only thing we can do is fall back to using the full image
		return opniImage, nil
	}

	return nil, fmt.Errorf("error resolving minimal image: %w", errors.Join(minimalImageErr, opniImageErr))
}

func init() {
	oci.RegisterFetcherBuilder("kubernetes", func(args ...any) (oci.Fetcher, error) {
		namespace := args[0].(string)

		var opts []KubernetesResolveImageDriverOption
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
