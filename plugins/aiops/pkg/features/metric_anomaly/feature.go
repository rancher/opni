package metric_anomaly

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nats-io/nats.go"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/metricai"
	"github.com/rancher/opni/plugins/aiops/pkg/features"
	"github.com/rancher/opni/plugins/aiops/pkg/features/metric_anomaly/drivers"
	"go.uber.org/zap"
)

const (
	metricAIJobBucket = "metric_ai_jobs"
	metricAIRunBucket = "metric_ai_job_runs"
)

type MetricAnomalyFeatureOptions struct {
	Logger                  *zap.SugaredLogger      `option:"logger"`
	DashboardDriver         drivers.DashboardDriver `option:"dashboardDriver"`
	MetricAnomalyServiceURL string                  `option:"metricAnomalyServiceURL"`
}

type MetricAnomaly struct {
	MetricAnomalyFeatureOptions
	metricai.UnsafeMetricAIServer

	ctx context.Context

	httpClient     *http.Client
	natsConnection future.Future[*nats.Conn]
	metricAIJobKv  future.Future[nats.KeyValue]
	metricAIRunKv  future.Future[nats.KeyValue]
}

func NewMetricAnomalyFeature(ctx context.Context, options MetricAnomalyFeatureOptions) (*MetricAnomaly, error) {
	return &MetricAnomaly{
		ctx:                         ctx,
		MetricAnomalyFeatureOptions: options,
		natsConnection:              future.New[*nats.Conn](),
		metricAIJobKv:               future.New[nats.KeyValue](),
		metricAIRunKv:               future.New[nats.KeyValue](),
		httpClient:                  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *MetricAnomaly) ManagementAPIExtensionServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService(&metricai.MetricAI_ServiceDesc, p),
	}
}

func (p *MetricAnomaly) UseManagementAPI(_ managementv1.ManagementClient) {
	lg := p.Logger
	nc, err := natsutil.AcquireNATSConnection(p.ctx)
	if err != nil {
		lg.Fatal(err)
	}
	mgr, err := nc.JetStream()
	if err != nil {
		lg.Fatal(err)
	}

	lg.Debug("connected to nats")

	metricAIJobKeyValue, err := mgr.KeyValue(metricAIJobBucket)
	if errors.Is(err, nats.ErrBucketNotFound) {
		metricAIJobKeyValue, err = mgr.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      metricAIJobBucket,
			Description: "Storing the submitted jobs for metric AI service.",
		})
	}
	if err != nil {
		lg.Fatal(err)
	}
	p.metricAIJobKv.Set(metricAIJobKeyValue)

	metricAIRunKeyValue, err := mgr.KeyValue(metricAIRunBucket)
	if errors.Is(err, nats.ErrBucketNotFound) {
		metricAIRunKeyValue, err = mgr.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      metricAIRunBucket,
			Description: "Storing the submitted jobs arun results for metric AI service.",
		})
	}
	if err != nil {
		lg.Fatal(err)
	}
	p.metricAIRunKv.Set(metricAIRunKeyValue)

	p.natsConnection.Set(nc)

	lg.Debug("initialization completed")

	<-p.ctx.Done()
}

func init() {
	features.Features.Register("metric-anomaly", func(ctx context.Context, opts ...driverutil.Option) (features.Feature, error) {
		options := MetricAnomalyFeatureOptions{
			MetricAnomalyServiceURL: "http://metric-ai-service:8090",
		}
		if err := driverutil.ApplyOptions(&options, opts...); err != nil {
			return nil, err
		}

		if options.DashboardDriver == nil {
			dashboardDrivers := drivers.DashboardDrivers.List()
			if len(dashboardDrivers) == 0 {
				return nil, fmt.Errorf("no dashboard drivers available")
			}
			builder, _ := drivers.DashboardDrivers.Get(dashboardDrivers[0])

			driver, err := builder(ctx, opts...)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize dashboard driver: %w", err)
			}
			options.DashboardDriver = driver
			options.Logger.Infof("using dashboard driver: %q", dashboardDrivers[0])
		}

		return NewMetricAnomalyFeature(ctx, options)
	})
}
