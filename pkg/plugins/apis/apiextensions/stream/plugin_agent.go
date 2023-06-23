package stream

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/go-plugin"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/kralicky/totem"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	CorrelationIDHeader = "x-correlation-id"
)

var (
	discoveryTimeout = 10 * time.Second
)

func NewAgentPlugin(p StreamAPIExtension) plugin.Plugin {
	pc, _, _, ok := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	name := "unknown"
	if ok {
		fnName := fn.Name()
		name = fnName[strings.LastIndex(fnName, "plugins/")+len("plugins/") : strings.LastIndex(fnName, ".")]
	}

	ext := &agentStreamExtensionServerImpl{
		name:          name,
		logger:        logger.NewPluginLogger().Named(name).Named("stream"),
		activeStreams: make(map[string]chan struct{}),
	}
	if p != nil {
		servers := p.StreamServers()
		for _, srv := range servers {
			descriptor, err := grpcreflect.LoadServiceDescriptor(srv.Desc)
			if err != nil {
				panic(err)
			}
			ext.servers = append(ext.servers, &richServer{
				Server:   srv,
				richDesc: descriptor,
			})
		}
		if clientHandler, ok := p.(StreamClientHandler); ok {
			ext.clientHandler = clientHandler
		}
	}
	return &streamApiExtensionPlugin[*agentStreamExtensionServerImpl]{
		extensionSrv: ext,
	}
}

type agentStreamExtensionServerImpl struct {
	streamv1.UnsafeStreamServer
	apiextensions.UnimplementedStreamAPIExtensionServer

	name          string
	servers       []*richServer
	clientHandler StreamClientHandler
	logger        *zap.SugaredLogger

	activeStreamsMu sync.Mutex
	activeStreams   map[string]chan struct{}
}

// Implements streamv1.StreamServer
func (e *agentStreamExtensionServerImpl) Connect(stream streamv1.Stream_ConnectServer) error {
	e.logger.Debug("stream connected")
	correlationId := uuid.NewString()
	stream.SetHeader(metadata.Pairs(CorrelationIDHeader, correlationId))

	e.activeStreamsMu.Lock()
	discoveryC := make(chan struct{}, 1)
	e.activeStreams[correlationId] = discoveryC
	e.activeStreamsMu.Unlock()
	defer func() {
		e.activeStreamsMu.Lock()
		delete(e.activeStreams, correlationId)
		e.activeStreamsMu.Unlock()
	}()

	opts := []totem.ServerOption{
		totem.WithName("plugin_" + e.name),
	}
	ts, err := totem.NewServer(stream, opts...)

	if err != nil {
		e.logger.With(
			zap.Error(err),
		).Error("failed to create stream server")
		return err
	}
	for _, srv := range e.servers {
		ts.RegisterService(srv.Desc, srv.Impl)
	}

	cc, errC := ts.Serve()

	e.logger.Debug("stream server started")

	select {
	case err := <-errC:
		if errors.Is(err, io.EOF) {
			return status.Errorf(codes.Aborted, "stream disconnected while waiting for discovery")
		} else {
			return status.Errorf(codes.Internal, "stream encountered an error while waiting for discovery: %v", err)
		}
	case <-discoveryC:
		e.logger.Debug("stream client is now available")
		if e.clientHandler != nil {
			e.clientHandler.UseStreamClient(cc)
		}
	case <-time.After(discoveryTimeout):
		// If we don't get a discovery event within 10 seconds, something went
		// wrong. To prevent the connection from hanging forever, close the stream
		// and reconnect.
		return status.Errorf(codes.DeadlineExceeded, "stream client discovery timed out after %s", discoveryTimeout)
	}

	err = <-errC
	if errors.Is(err, io.EOF) {
		e.logger.Debug("stream disconnected")
	} else {
		e.logger.With(
			zap.Error(err),
		).Warn("stream disconnected with error")
	}
	return err
}

func (e *agentStreamExtensionServerImpl) Notify(ctx context.Context, event *streamv1.StreamEvent) (*emptypb.Empty, error) {
	e.logger.With(
		"type", event.Type.String(),
	).Debugf("received notify event for '%s'", e.name)
	e.activeStreamsMu.Lock()
	defer e.activeStreamsMu.Unlock()

	if event.Type == streamv1.EventType_DiscoveryComplete {
		e.logger.Debug("processing discovery complete event")

		correlationId := event.GetCorrelationId()
		if correlationId == "" {
			// backwards compatibility:
			// if correlation ID is not set, just notify all active streams.
			// the id is only to prevent rare timing issues anyway; there is still
			// only one active stream at a time.
			for _, c := range e.activeStreams {
				select {
				case c <- struct{}{}:
				default:
				}
			}
		} else {
			select {
			case e.activeStreams[correlationId] <- struct{}{}:
			default:
			}
		}
	}
	return &emptypb.Empty{}, nil
}
