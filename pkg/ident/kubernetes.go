package ident

import (
	"context"
	"log"

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
		log.Fatal(err)
	}
	cs, err := kubernetes.NewForConfig(rc)
	if err != nil {
		log.Fatal(err)
	}
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
