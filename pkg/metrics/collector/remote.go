package collector

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/rancher/opni-monitoring/pkg/metrics/desc"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type remoteCollector struct {
	client RemoteCollectorClient
}

var _ prometheus.Collector = (*remoteCollector)(nil)

func NewRemoteCollector(client RemoteCollectorClient) prometheus.Collector {
	return &remoteCollector{
		client: client,
	}
}

func (rc *remoteCollector) Describe(c chan<- *prometheus.Desc) {
	ctx, ca := context.WithTimeout(context.Background(), 2*time.Second)
	defer ca()
	resp, err := rc.client.Describe(ctx, &emptypb.Empty{})
	if err != nil {
		c <- prometheus.NewInvalidDesc(err)
		return
	}
	for _, desc := range resp.GetDescriptors() {
		c <- desc.ToPrometheusDescUnsafe()
	}
}

func (rc *remoteCollector) Collect(c chan<- prometheus.Metric) {
	ctx, ca := context.WithTimeout(context.Background(), 2*time.Second)
	defer ca()
	resp, err := rc.client.Collect(ctx, &emptypb.Empty{})
	if err != nil {
		c <- prometheus.NewInvalidMetric(prometheus.NewInvalidDesc(err), err)
		return
	}
	for _, metric := range resp.GetMetrics() {
		c <- &remoteMetric{
			desc:   metric.GetDesc().ToPrometheusDescUnsafe(),
			metric: metric.GetMetric(),
		}
	}
}

type remoteMetric struct {
	desc   *prometheus.Desc
	metric *dto.Metric
}

var _ prometheus.Metric = (*remoteMetric)(nil)

func (rm *remoteMetric) Desc() *prometheus.Desc {
	return rm.desc
}

func (rm *remoteMetric) Write(m *dto.Metric) error {
	*m = *rm.metric
	return nil
}

type CollectorServer interface {
	RemoteCollectorServer
	MustRegister(collectors ...prometheus.Collector)
}

type remoteCollectorServer struct {
	UnimplementedRemoteCollectorServer

	collectors []prometheus.Collector
	lock       sync.RWMutex

	descChannelPool   sync.Pool
	metricChannelPool sync.Pool
}

func NewCollectorServer() CollectorServer {
	return &remoteCollectorServer{
		collectors: []prometheus.Collector{},
		descChannelPool: sync.Pool{
			New: func() interface{} {
				return make(chan *prometheus.Desc, 1000)
			},
		},
		metricChannelPool: sync.Pool{
			New: func() interface{} {
				return make(chan prometheus.Metric, 1000)
			},
		},
	}
}

func (s *remoteCollectorServer) MustRegister(collectors ...prometheus.Collector) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.collectors = append(s.collectors, collectors...)
}

func (s *remoteCollectorServer) Describe(ctx context.Context, _ *emptypb.Empty) (*DescriptorList, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	descs := []*desc.Desc{}
	c := s.descChannelPool.Get().(chan *prometheus.Desc)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, collector := range s.collectors {
			collector.Describe(c)
		}
	}()

READ:
	for {
		select {
		case d := <-c:
			descs = append(descs, desc.FromPrometheusDesc(d))
		case <-done:
			break READ
		}
	}

	return &DescriptorList{
		Descriptors: descs,
	}, nil
}

func (s *remoteCollectorServer) Collect(ctx context.Context, _ *emptypb.Empty) (*MetricList, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	metrics := []*Metric{}
	c := s.metricChannelPool.Get().(chan prometheus.Metric)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, collector := range s.collectors {
			collector.Collect(c)
		}
	}()

READ:
	for {
		select {
		case metric := <-c:
			m := &Metric{
				Desc:   desc.FromPrometheusDesc(metric.Desc()),
				Metric: &dto.Metric{},
			}
			metric.Write(m.Metric)
			metrics = append(metrics, m)
		case <-done:
			break READ
		}
	}

	return &MetricList{
		Metrics: metrics,
	}, nil
}
