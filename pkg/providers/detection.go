package providers

import (
	"context"
	"log"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provider int

const (
	Unknown Provider = iota
	K3S
	RKE2
)

func Detect(c client.Client) Provider {
	nodes := &corev1.NodeList{}
	if err := c.List(context.Background(), nodes); err != nil {
		log.Fatal(err)
	}
	for _, node := range nodes.Items {
		if strings.Contains(node.Spec.ProviderID, "k3s") {
			return K3S
		} else if strings.Contains(node.Spec.ProviderID, "rke2") {
			return RKE2
		}
	}
	return Unknown
}
