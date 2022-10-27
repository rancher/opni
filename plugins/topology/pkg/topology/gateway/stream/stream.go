package stream

/*
Implements logic to push topology data to the remote endpoint (gateway).

The remote write server lives on the gateway and agents acquire a client
to stream to.
*/

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/nats-io/nats.go"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/topology/store"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/topology/pkg/apis/remote"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TopologyRemoteWriteConfig struct {
	Logger *zap.SugaredLogger
	Nc     *nats.Conn
}

type TopologyRemoteWriter struct {
	remote.UnsafeRemoteTopologyServer
	TopologyRemoteWriteConfig

	topologyObjectStore nats.ObjectStore

	util.Initializer
}

var _ remote.RemoteTopologyServer = (*TopologyRemoteWriter)(nil)

func (t *TopologyRemoteWriter) Initialize(conf TopologyRemoteWriteConfig) {
	t.InitOnce(func() {
		objStore, err := store.NewTopologyObjectStore(conf.Nc)
		if err != nil {
			conf.Logger.With("error", err).Error("failed to initialize topology object store")
			os.Exit(1)
		}
		t.topologyObjectStore = objStore
	})
}

func (t *TopologyRemoteWriter) objectDef(clusterId *corev1.Reference, repr remote.GraphRepr) *nats.ObjectMeta {
	return &nats.ObjectMeta{
		Name: store.NewClusterKey(clusterId),
		Description: fmt.Sprintf(
			"topology for cluster %s, in representation form %s",
			clusterId.GetId(),
			repr.String()),
		Headers: nats.Header{
			store.ReprHeaderKey: []string{repr.String()},
		},
	}
}

func (t *TopologyRemoteWriter) Push(ctx context.Context, payload *remote.Payload) (*emptypb.Empty, error) {
	if !t.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}

	info, err := t.topologyObjectStore.Put(
		t.objectDef(payload.Graph.ClusterId, payload.Graph.Repr),
		bytes.NewReader(payload.Graph.Data),
	)
	if err != nil {
		return nil, err
	}
	t.Logger.With("info", info).Debug("successfully pushed topology data")
	return &emptypb.Empty{}, nil
}

func (t *TopologyRemoteWriter) SyncTopology(ctx context.Context, payload *remote.Payload) (*emptypb.Empty, error) {
	if !t.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}

	// TODO(topology) : implement me

	return nil, shared.WithUnimplementedError("method not implemented")
}
