package log_anomaly

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/nats-io/nats.go"
	"github.com/opensearch-project/opensearch-go"
	"github.com/rancher/opni/apis"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/pkg/util/k8sutil"
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	natsutil "github.com/rancher/opni/pkg/util/nats"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/admin"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"github.com/rancher/opni/plugins/aiops/pkg/features"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	opsterv1 "opensearch.opster.io/api/v1"
	"opensearch.opster.io/pkg/helpers"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	workloadAggregationCountBucket = "os-workload-aggregation"
	modelTrainingParametersBucket  = "model-training-parameters"
	modelTrainingStatisticsBucket  = "model-training-statistics"
)

type LogAnomalyOptions struct {
	StorageNamespace  string                         `option:"namespace"`
	Version           string                         `option:"version"`
	OpensearchCluster *opnimeta.OpensearchClusterRef `option:"opensearchCluster"`
	K8sClient         client.Client                  `option:"k8sClient"`
	Logger            *zap.SugaredLogger             `option:"logger"`
}

type LogAnomaly struct {
	LogAnomalyOptions
	modeltraining.UnsafeModelTrainingServer
	admin.UnsafeAIAdminServer

	ctx context.Context

	osClient        future.Future[*opensearch.Client]
	natsConnection  future.Future[*nats.Conn]
	aggregationKv   future.Future[nats.KeyValue]
	modelTrainingKv future.Future[nats.KeyValue]
	statisticsKv    future.Future[nats.KeyValue]
}

func (p *LogAnomaly) ManagementAPIExtensionServices() []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService(&modeltraining.ModelTraining_ServiceDesc, p),
		util.PackService(&admin.AIAdmin_ServiceDesc, p),
	}
}

func (p *LogAnomaly) UseManagementAPI(managementClient managementv1.ManagementClient) {
	lg := p.Logger
	nc, err := natsutil.AcquireNATSConnection(p.ctx)
	if err != nil {
		lg.Fatal(err)
	}
	mgr, err := nc.JetStream()
	if err != nil {
		lg.Fatal(err)
	}
	aggregationKeyValue, err := mgr.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      workloadAggregationCountBucket,
		Description: "Storing aggregation of workload logs from Opensearch.",
	})
	if err != nil {
		lg.Fatal(err)
	}

	p.aggregationKv.Set(aggregationKeyValue)

	modelParametersKeyValue, err := mgr.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      modelTrainingParametersBucket,
		Description: "Storing the workloads specified for model training.",
	})
	if err != nil {
		lg.Fatal(err)
	}

	p.modelTrainingKv.Set(modelParametersKeyValue)

	modelStatisticsKeyValue, err := mgr.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      modelTrainingStatisticsBucket,
		Description: "Storing the statistics for the training of the model.",
	})
	if err != nil {
		lg.Fatal(err)
	}

	p.statisticsKv.Set(modelStatisticsKeyValue)

	p.natsConnection.Set(nc)

	go p.runAggregation()
	go p.setOpensearchConnection()
	<-p.ctx.Done()
}

func (s *LogAnomaly) setOpensearchConnection() {
	esEndpoint := fmt.Sprintf("https://opni-opensearch-svc.%s.svc:9200", s.StorageNamespace)
	retrier := backoffv2.Exponential(
		backoffv2.WithMaxRetries(0),
		backoffv2.WithMinInterval(5*time.Second),
		backoffv2.WithMaxInterval(1*time.Minute),
		backoffv2.WithMultiplier(1.1),
	)
	b := retrier.Start(s.ctx)
	cluster := &opsterv1.OpenSearchCluster{}
FETCH:
	for {
		select {
		case <-b.Done():
			s.Logger.Warn("plugin context cancelled before Opensearch object created")
		case <-b.Next():
			err := s.K8sClient.Get(s.ctx, types.NamespacedName{
				Name:      "opni",
				Namespace: s.StorageNamespace,
			}, cluster)
			if err != nil {
				if k8serrors.IsNotFound(err) {
					s.Logger.Info("waiting for k8s object")
					continue
				}
				s.Logger.Errorf("failed to check k8s object: %v", err)
				continue
			}
			break FETCH
		}
	}
	esUsername, esPassword, err := helpers.UsernameAndPassword(s.ctx, s.K8sClient, cluster)
	if err != nil {
		panic(err)
	}
	osClient, err := opensearch.NewClient(opensearch.Config{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Addresses: []string{esEndpoint},
		Username:  esUsername,
		Password:  esPassword,
	})
	if err != nil {
		panic(err)
	}

	s.osClient.Set(osClient)
}

func init() {
	features.Features.Register("log-anomaly", func(ctx context.Context, opts ...driverutil.Option) (features.Feature, error) {
		options := LogAnomalyOptions{
			StorageNamespace: os.Getenv("POD_NAMESPACE"),
			OpensearchCluster: &opnimeta.OpensearchClusterRef{
				Name:      "opni",
				Namespace: os.Getenv("POD_NAMESPACE"),
			},
			Version: "v0.9.2",
		}
		if err := driverutil.ApplyOptions(&options, opts...); err != nil {
			return nil, err
		}

		if options.K8sClient == nil {
			c, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
				Scheme: apis.NewScheme(),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
			}
			options.K8sClient = c
		}

		return &LogAnomaly{
			ctx:               ctx,
			LogAnomalyOptions: options,
			aggregationKv:     future.New[nats.KeyValue](),
			modelTrainingKv:   future.New[nats.KeyValue](),
			statisticsKv:      future.New[nats.KeyValue](),
			osClient:          future.New[*opensearch.Client](),
		}, nil
	})
}
