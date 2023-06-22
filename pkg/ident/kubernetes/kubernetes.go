package kubernetes

import (
	"context"

	"github.com/rancher/opni/pkg/ident"
	"github.com/rancher/opni/pkg/util"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
)

type kubernetesProvider struct {
	KubernetesIdentOptions
	restClient *rest.RESTClient
}

type KubernetesIdentOptions struct {
	restConfig *rest.Config
}

type KubernetesIdentOption func(*KubernetesIdentOptions)

func (o *KubernetesIdentOptions) apply(opts ...KubernetesIdentOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithRestConfig(rc *rest.Config) KubernetesIdentOption {
	return func(o *KubernetesIdentOptions) {
		o.restConfig = rc
	}
}

func NewKubernetesProvider(opts ...KubernetesIdentOption) ident.Provider {
	options := KubernetesIdentOptions{}
	options.apply(opts...)
	if options.restConfig == nil {
		options.restConfig = util.Must(rest.InClusterConfig())
	}

	scheme := runtime.NewScheme()
	util.Must(corev1.AddToScheme(scheme))
	options.restConfig.NegotiatedSerializer = serializer.NewCodecFactory(scheme)

	return &kubernetesProvider{
		restClient: util.Must(rest.UnversionedRESTClientFor(options.restConfig)),
	}
}

func (p *kubernetesProvider) UniqueIdentifier(ctx context.Context) (string, error) {
	var ns corev1.Namespace
	err := p.restClient.Get().AbsPath("/api/v1/namespaces/kube-system").Do(ctx).Into(&ns)
	if err != nil {
		return "", err
	}
	return string(ns.GetUID()), nil
}

func init() {
	util.Must(ident.RegisterProvider("kubernetes", func(_ ...any) ident.Provider {
		return NewKubernetesProvider()
	}))
}
