package gateway

import (
	"context"
	"os"

	"github.com/nats-io/nats.go"
	opensearch "github.com/opensearch-project/opensearch-go"
	"github.com/rancher/opni/apis"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/admin"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AIOpsPlugin struct {
	modeltraining.UnsafeModelTrainingServer
	system.UnimplementedSystemPluginClient
	ctx              context.Context
	Logger           *zap.SugaredLogger
	k8sClient        client.Client
	osClient         future.Future[*opensearch.Client]
	natsConnection   future.Future[*nats.Conn]
	kv               future.Future[nats.KeyValue]
	storageNamespace string
}

func (s *AIOpsPlugin) UseManagementAPI(api managementv1.ManagementClient) {
	lg := s.Logger
	nc, err := newNatsConnection()
	if err != nil {
		lg.Fatal(err)
	}
	mgr, err := nc.JetStream()
	if err != nil {
		lg.Fatal(err)
	}
	keyValue, err := mgr.CreateKeyValue(&nats.KeyValueConfig{
		Bucket:      "os-workload-aggregation",
		Description: "Storing aggregation of workload logs from Opensearch.",
	})
	if err != nil {
		lg.Fatal(err)
	}

	s.natsConnection.Set(nc)
	s.kv.Set(keyValue)

	go s.runAggregation()
	<-s.ctx.Done()
}

var _ modeltraining.ModelTrainingServer = (*AIOpsPlugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := &AIOpsPlugin{
		Logger:         logger.NewPluginLogger().Named("modeltraining"),
		ctx:            ctx,
		natsConnection: future.New[*nats.Conn](),
		kv:             future.New[nats.KeyValue](),
		osClient:       future.New[*opensearch.Client](),
	}

	p.storageNamespace = os.Getenv("POD_NAMESPACE")

	k8sClient, err := k8sutil.NewK8sClient(k8sutil.ClientOptions{
		Scheme: apis.NewScheme(),
	})
	if err != nil {
		p.Logger.Fatal(err)
	}
	p.k8sClient = k8sClient

	go p.setOpensearchConnection()

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(
		managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(
			util.PackService(&modeltraining.ModelTraining_ServiceDesc, p),
			util.PackService(&admin.Admin_ServiceDesc, p),
		),
	)
	return scheme
}
