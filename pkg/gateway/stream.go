package gateway

import (
	"errors"
	"fmt"
	"sync"

	"github.com/kralicky/totem"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/metrics"
	"go.opentelemetry.io/otel/attribute"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	agentv1 "github.com/rancher/opni/pkg/agent"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	streamv1 "github.com/rancher/opni/pkg/apis/stream/v1"
	"github.com/rancher/opni/pkg/auth/cluster"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

type remote struct {
	name string
	cc   *grpc.ClientConn
}

type StreamServer struct {
	streamv1.UnimplementedStreamServer
	logger            *zap.SugaredLogger
	handler           ConnectionHandler
	clusterStore      storage.ClusterStore
	services          []util.ServicePack[any]
	remotesMu         sync.Mutex
	remotes           []remote
	metricsRegisterer prometheus.Registerer

	providersMu  sync.Mutex
	providerById map[string]*metric.MeterProvider
}

func NewStreamServer(
	handler ConnectionHandler,
	clusterStore storage.ClusterStore,
	metricsRegisterer prometheus.Registerer,
	lg *zap.SugaredLogger,
) *StreamServer {

	return &StreamServer{
		logger:            lg.Named("grpc"),
		handler:           handler,
		clusterStore:      clusterStore,
		metricsRegisterer: metricsRegisterer,
		providerById:      make(map[string]*metric.MeterProvider),
	}
}

func (s *StreamServer) getProviderForId(agentId string) *metric.MeterProvider {
	s.providersMu.Lock()
	defer s.providersMu.Unlock()
	if prev, ok := s.providerById[agentId]; ok {
		return prev
	}
	exporter, err := otelprometheus.New(
		otelprometheus.WithRegisterer(prometheus.WrapRegistererWithPrefix("opni_gateway_", s.metricsRegisterer)),
		otelprometheus.WithoutScopeInfo(),
		otelprometheus.WithoutTargetInfo(),
	)
	if err != nil {
		s.logger.With(zap.Error(err)).Panic("failed to initialize stream metrics exporter")
	}

	provider := metric.NewMeterProvider(metric.WithReader(exporter),
		metric.WithResource(resource.NewSchemaless(attribute.Key("agent-id").String(agentId))))
	s.providerById[agentId] = provider
	return provider
}

func (s *StreamServer) Connect(stream streamv1.Stream_ConnectServer) error {
	s.logger.Debug("handling new stream connection")
	ctx := stream.Context()

	id := cluster.StreamAuthorizedID(ctx)

	opts := []totem.ServerOption{
		totem.WithName("gateway-server"),
	}
	opts = append(opts, totem.WithMetrics(s.getProviderForId(id),
		attribute.Key(metrics.LabelImpersonateAs).String(id),
	))

	ts, err := totem.NewServer(stream, opts...)
	if err != nil {
		return err
	}
	for _, service := range s.services {
		ts.RegisterService(service.Unpack())
	}

	c, err := s.clusterStore.GetCluster(ctx, &corev1.Reference{
		Id: id,
	})
	if err != nil {
		s.logger.With(
			zap.Error(err),
			"id", id,
		).Error("failed to get cluster")
		return err
	}
	eventC, err := s.clusterStore.WatchCluster(ctx, c)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	ctx = storage.NewWatchContext(ctx, eventC)

	for _, r := range s.remotes {
		streamClient := streamv1.NewStreamClient(r.cc)
		splicedStream, err := streamClient.Connect(ctx)
		if err != nil {
			s.logger.With(
				zap.String("clusterId", c.Id),
				zap.Error(err),
			).Warn("failed to connect to remote stream, skipping")
			continue
		}
		if err := ts.Splice(splicedStream, totem.WithStreamName(r.name)); err != nil {
			s.logger.With(
				zap.String("clusterId", c.Id),
				zap.Error(err),
			).Warn("failed to splice remote stream, skipping")
			continue
		}
	}

	cc, errC := ts.Serve()

	// check if an error was immediately returned
	select {
	case err := <-errC:
		return fmt.Errorf("stream connection failed: %w", err)
	default:
	}

	go s.handler.HandleAgentConnection(ctx, agentv1.NewClientSet(cc))

	select {
	case err = <-errC:
		if err != nil {
			s.logger.With(
				zap.Error(err),
			).Warn("agent stream disconnected")
		}
		return status.Error(codes.Unavailable, err.Error())
	case <-ctx.Done():
		s.logger.With(
			zap.Error(ctx.Err()),
		).Info("agent stream closing")
		err := ctx.Err()
		if errors.Is(err, storage.ErrObjectDeleted) {
			return status.Error(codes.Unauthenticated, err.Error())
		}
		return status.Error(codes.Unavailable, err.Error())
	}
}

func (s *StreamServer) RegisterService(desc *grpc.ServiceDesc, impl any) {
	s.logger.With(
		zap.String("service", desc.ServiceName),
	).Debug("registering service")
	if len(desc.Streams) > 0 {
		s.logger.With(
			zap.String("service", desc.ServiceName),
		).Panic("failed to register service: nested streams are currently not supported")
	}
	s.services = append(s.services, util.PackService(desc, impl))
}

func (s *StreamServer) OnPluginLoad(_ types.StreamAPIExtensionPlugin, md meta.PluginMeta, cc *grpc.ClientConn) {
	s.remotesMu.Lock()
	defer s.remotesMu.Unlock()
	s.logger.With(
		zap.String("address", cc.Target()),
	).Debug("adding remote connection")
	s.remotes = append(s.remotes, remote{
		name: md.Filename(),
		cc:   cc,
	})
}
