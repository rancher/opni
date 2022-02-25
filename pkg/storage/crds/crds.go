package crds

import (
	"github.com/rancher/opni-monitoring/pkg/storage"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type CRDStore struct {
	client    client.Client
	namespace string
}

var _ storage.TokenStore = (*CRDStore)(nil)
var _ storage.ClusterStore = (*CRDStore)(nil)
var _ storage.RBACStore = (*CRDStore)(nil)
