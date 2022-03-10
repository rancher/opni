package ident

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kubernetesProvider struct {
	KubernetesIdentOptions
	clientset *kubernetes.Clientset
}

type KubernetesIdentOptions struct {
	restConfig *rest.Config
}

type KubernetesIdentOption func(*KubernetesIdentOptions)

func (o *KubernetesIdentOptions) Apply(opts ...KubernetesIdentOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithRestConfig(rc *rest.Config) KubernetesIdentOption {
	return func(o *KubernetesIdentOptions) {
		o.restConfig = rc
	}
}

func NewKubernetesProvider(opts ...KubernetesIdentOption) Provider {
	options := KubernetesIdentOptions{}
	options.Apply(opts...)
	if options.restConfig == nil {
		rc, err := rest.InClusterConfig()
		if err != nil {
			panic(err)
		}
		options.restConfig = rc
	}
	cs := kubernetes.NewForConfigOrDie(options.restConfig)
	return &kubernetesProvider{
		clientset: cs,
	}
}

func (p *kubernetesProvider) UniqueIdentifier(ctx context.Context) (string, error) {
	ns, err := p.clientset.CoreV1().
		Namespaces().
		Get(ctx, "kube-system", metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	return string(ns.GetUID()), nil
}

func init() {
	if err := RegisterProvider("kubernetes", func() Provider {
		return NewKubernetesProvider()
	}); err != nil {
		panic(err)
	}
}
