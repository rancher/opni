package store

import (
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
)

const (
	ReprHeaderKey         = "repr"
	TopologyObjectStoreId = "downstream-topology"
)

func NewTopologyObjectStore(nc *nats.Conn) (nats.ObjectStore, error) {
	mgr, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	if obj, err := mgr.ObjectStore(TopologyObjectStoreId); errors.Is(err, nats.ErrStreamNotFound) {
		objSt, err := mgr.CreateObjectStore(&nats.ObjectStoreConfig{
			Bucket:      TopologyObjectStoreId,
			Description: "Store downstream kubernetes topology, indexed by cluster id",
		})
		if err != nil {
			return nil, err
		}
		return objSt, nil
	} else if err != nil {
		return nil, err
	} else {
		return obj, nil
	}
}

func GetTopologyObjectStore(nc *nats.Conn) (nats.ObjectStore, error) {
	mgr, err := nc.JetStream()
	if err != nil {
		return nil, err
	}
	obj, err := mgr.ObjectStore(TopologyObjectStoreId)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func NewClusterKey(clusterId *corev1.Reference) string {
	return fmt.Sprintf("topology-%s", clusterId.GetId())
}
