package ident

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kubernetesProvider struct {
	clientset *kubernetes.Clientset
}

func NewKubernetesProvider() Provider {
	rc, err := rest.InClusterConfig()
	if err != nil {
		panic(err)
	}
	cs := kubernetes.NewForConfigOrDie(rc)
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
	if err := RegisterProvider("kubernetes", NewKubernetesProvider); err != nil {
		panic(err)
	}
}
