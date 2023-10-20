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
	"github.com/rancher/opni/plugins/topology/apis/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
	"log/slog"
)

type TopologyStreamWriteConfig struct {
	Logger *slog.Logger
	Nc     *nats.Conn
}

type TopologyStreamWriter struct {
	stream.UnsafeRemoteTopologyServer
	TopologyStreamWriteConfig

	topologyObjectStore nats.ObjectStore

	util.Initializer
}

var _ stream.RemoteTopologyServer = (*TopologyStreamWriter)(nil)

func (t *TopologyStreamWriter) Initialize(conf TopologyStreamWriteConfig) {
	t.InitOnce(func() {
		objStore, err := store.NewTopologyObjectStore(conf.Nc)
		if err != nil {
			conf.Logger.With("error", err).Error("failed to initialize topology object store")
			os.Exit(1)
		}
		t.topologyObjectStore = objStore
		t.TopologyStreamWriteConfig = conf
	})
}

func (t *TopologyStreamWriter) objectDef(clusterId *corev1.Reference, repr stream.GraphRepr) *nats.ObjectMeta {
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

func (t *TopologyStreamWriter) Push(_ context.Context, payload *stream.Payload) (*emptypb.Empty, error) {
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

func (t *TopologyStreamWriter) SyncTopology(_ context.Context, _ *stream.Payload) (*emptypb.Empty, error) {
	if !t.Initialized() {
		return nil, util.StatusError(codes.Unavailable)
	}

	// TODO(topology) : implement me

	return nil, shared.WithUnimplementedError("method not implemented")
}
