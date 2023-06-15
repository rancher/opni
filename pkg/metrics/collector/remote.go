package collector

import (
	"context"
	"sync"

	"github.com/rancher/opni/pkg/metrics/collector/transform"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	otlpmetricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type RemoteProducer interface {
	metric.Producer
}

type remoteProducer struct {
	client RemoteCollectorClient
}

var _ metric.Producer = (*remoteProducer)(nil)

func NewRemoteProducer(client RemoteCollectorClient) RemoteProducer {
	return &remoteProducer{
		client: client,
	}
}

func (rc *remoteProducer) Produce(ctx context.Context) ([]metricdata.ScopeMetrics, error) {
	metrics, err := rc.client.GetMetrics(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}

	out := []metricdata.ScopeMetrics{}
	for _, resourceMetric := range metrics.GetResourceMetrics() {
		scopeMetrics, err := transform.ProtoScopeMetrics(resourceMetric.GetScopeMetrics())
		if err != nil {
			return out, err
		}
		out = append(out, scopeMetrics...)
	}
	return out, nil
}

type CollectorServer interface {
	RemoteCollectorServer
	AppendReader(metric.Reader)
}

type remoteCollectorServer struct {
	UnsafeRemoteCollectorServer
	readers []metric.Reader
	rMutex  sync.Mutex
}

func NewCollectorServer(readers ...metric.Reader) CollectorServer {
	return &remoteCollectorServer{
		readers: readers,
	}
}

func (s *remoteCollectorServer) GetMetrics(ctx context.Context, _ *emptypb.Empty) (*otlpmetricsv1.MetricsData, error) {
	s.rMutex.Lock()
	defer s.rMutex.Unlock()
	rms := make([]*otlpmetricsv1.ResourceMetrics, len(s.readers))
	for i, reader := range s.readers {
		metrics := &metricdata.ResourceMetrics{}
		err := reader.Collect(ctx, metrics)
		if err != nil {
			return nil, err
		}

		metricspb, err := transform.ResourceMetrics(metrics)
		if err != nil {
			return nil, err
		}

		rms[i] = metricspb
	}

	return &otlpmetricsv1.MetricsData{
		ResourceMetrics: rms,
	}, nil
}

func (s *remoteCollectorServer) AppendReader(reader metric.Reader) {
	s.rMutex.Lock()
	defer s.rMutex.Unlock()
	s.readers = append(s.readers, reader)
}
