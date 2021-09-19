package v1

import (
	nfdv1 "github.com/kubernetes-sigs/node-feature-discovery-operator/api/v1"
)

func init() {
	SchemeBuilder.Register(
		&nfdv1.NodeFeatureDiscovery{}, &nfdv1.NodeFeatureDiscoveryList{},
	)
}
