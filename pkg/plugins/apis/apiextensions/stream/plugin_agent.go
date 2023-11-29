package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
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
	"github.com/samber/lo"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.uber.org/atomic"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	CorrelationIDHeader = "x-correlation-id"
)

var (
	discoveryTimeout = atomic.NewDuration(10 * time.Second)
)

func NewAgentPlugin(ctx context.Context, p StreamAPIExtension) plugin.Plugin {
	pc, _, _, ok := runtime.Caller(1)
	fn := runtime.FuncForPC(pc)
	name := "unknown"
	if ok {
		fnName := fn.Name()
		parts := strings.Split(fnName, "/")
		name = fmt.Sprintf("plugin_%s", parts[slices.Index(parts, "plugins")+1])
	}

	lg := logger.NewPluginLogger(ctx).WithGroup(name).WithGroup("stream")
	ctx = logger.WithPluginLogger(ctx, lg)

	ext := &agentStreamExtensionServerImpl{
		ctx:           ctx,
		name:          name,
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

	ctx           context.Context
	name          string
	servers       []*richServer
	clientHandler StreamClientHandler

	activeStreamsMu sync.Mutex
	activeStreams   map[string]chan struct{}
}

// Implements streamv1.StreamServer
func (e *agentStreamExtensionServerImpl) Connect(stream streamv1.Stream_ConnectServer) error {
	lg := logger.PluginLoggerFromContext(e.ctx)
	lg.Debug("stream connected")
	correlationId := uuid.NewString()
	stream.SendHeader(metadata.Pairs(CorrelationIDHeader, correlationId))

	e.activeStreamsMu.Lock()
	notifyC := make(chan struct{}, 1)
	e.activeStreams[correlationId] = notifyC
	e.activeStreamsMu.Unlock()
	defer func() {
		e.activeStreamsMu.Lock()
		delete(e.activeStreams, correlationId)
		e.activeStreamsMu.Unlock()
	}()

	opts := []totem.ServerOption{
		totem.WithName(e.name),
		totem.WithTracerOptions(
			resource.WithAttributes(
				semconv.ServiceNameKey.String(e.name),
				attribute.String("mode", "agent"),
			),
		),
	}
	ts, err := totem.NewServer(stream, opts...)

	if err != nil {
		lg.With(
			logger.Err(err),
		).Error("failed to create stream server")
		return err
	}
	for _, srv := range e.servers {
		ts.RegisterService(srv.Desc, srv.Impl)
	}
	timeout := discoveryTimeout.Load()
	var cc grpc.ClientConnInterface
	var errC <-chan error
	select {
	case res := <-lo.Async2(ts.Serve):
		cc, errC = res.Unpack()
		if cc == nil {
			select {
			case err := <-errC:
				return fmt.Errorf("stream server failed to start: %w", err)
			default:
				return fmt.Errorf("stream server failed to start (unknown error)")
			}
		}
	case <-time.After(timeout):
		// If we don't get a discovery event within 10 seconds, something went
		// wrong. To prevent the connection from hanging forever, close the stream
		// and reconnect.
		return status.Errorf(codes.DeadlineExceeded, "stream client discovery timed out after %s", timeout)
	case <-stream.Context().Done():
		lg.With(stream.Context().Err()).Error("stream disconnected while waiting for discovery")
		return stream.Context().Err()
	}

	select {
	case <-notifyC:
		lg.Debug("stream client is now available")
		if e.clientHandler != nil {
			e.clientHandler.UseStreamClient(cc)
		}
	case err := <-errC:
		if err != nil {
			lg.With(stream.Context().Err()).Error("stream encountered an error while waiting for discovery")
			return status.Errorf(codes.Internal, "stream encountered an error while waiting for discovery: %v", err)
		}
	}

	lg.Debug("stream server started")

	err = <-errC
	if errors.Is(err, io.EOF) {
		lg.Debug("stream disconnected")
	} else if status.Code(err) == codes.Canceled {
		lg.Debug("stream closed")
	} else {
		lg.With(
			logger.Err(err),
		).Warn("stream disconnected with error")
	}
	return err
}

func (e *agentStreamExtensionServerImpl) Notify(_ context.Context, event *streamv1.StreamEvent) (*emptypb.Empty, error) {
	lg := logger.PluginLoggerFromContext(e.ctx)
	lg.With(
		"type", event.Type.String(),
	).Debug(fmt.Sprintf("received notify event for '%s'", e.name))
	e.activeStreamsMu.Lock()
	defer e.activeStreamsMu.Unlock()

	if event.Type == streamv1.EventType_DiscoveryComplete {
		lg.Debug("processing discovery complete event")

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
