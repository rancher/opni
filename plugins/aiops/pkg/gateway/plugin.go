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
	opnimeta "github.com/rancher/opni/pkg/util/meta"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/admin"
	"github.com/rancher/opni/plugins/aiops/pkg/apis/modeltraining"
	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AIOpsPlugin struct {
	PluginOptions
	modeltraining.UnsafeModelTrainingServer
	admin.UnsafeAIAdminServer
	system.UnimplementedSystemPluginClient
	ctx            context.Context
	Logger         *zap.SugaredLogger
	k8sClient      client.Client
	osClient       future.Future[*opensearch.Client]
	natsConnection future.Future[*nats.Conn]
	kv             future.Future[nats.KeyValue]
}

type PluginOptions struct {
	storageNamespace  string
	version           string
	opensearchCluster *opnimeta.OpensearchClusterRef
	restconfig        *rest.Config
}

type PluginOption func(*PluginOptions)

func (o *PluginOptions) apply(opts ...PluginOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithNamespace(namespace string) PluginOption {
	return func(o *PluginOptions) {
		o.storageNamespace = namespace
	}
}

func WithVersion(version string) PluginOption {
	return func(o *PluginOptions) {
		o.version = version
	}
}

func WithOpensearchCluster(cluster *opnimeta.OpensearchClusterRef) PluginOption {
	return func(o *PluginOptions) {
		o.opensearchCluster = cluster
	}
}

func WithRestConfig(restconfig *rest.Config) PluginOption {
	return func(o *PluginOptions) {
		o.restconfig = restconfig
	}
}

func NewPlugin(ctx context.Context, opts ...PluginOption) *AIOpsPlugin {
	options := PluginOptions{
		storageNamespace: os.Getenv("POD_NAMESPACE"),
		opensearchCluster: &opnimeta.OpensearchClusterRef{
			Name:      "opni",
			Namespace: os.Getenv("POD_NAMESPACE"),
		},
		version: "v0.8.2",
	}
	options.apply(opts...)

	var restconfig *rest.Config
	if options.restconfig != nil {
		restconfig = options.restconfig
	} else {
		restconfig = ctrl.GetConfigOrDie()
	}

	cli, err := client.New(restconfig, client.Options{
		Scheme: apis.NewScheme(),
	})
	if err != nil {
		panic(err)
	}

	return &AIOpsPlugin{
		PluginOptions:  options,
		Logger:         logger.NewPluginLogger().Named("modeltraining"),
		ctx:            ctx,
		natsConnection: future.New[*nats.Conn](),
		kv:             future.New[nats.KeyValue](),
		osClient:       future.New[*opensearch.Client](),
		k8sClient:      cli,
	}
}

func (p *AIOpsPlugin) UseManagementAPI(_ managementv1.ManagementClient) {
	lg := p.Logger
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

	p.natsConnection.Set(nc)
	p.kv.Set(keyValue)

	go p.runAggregation()
	<-p.ctx.Done()
}

var _ modeltraining.ModelTrainingServer = (*AIOpsPlugin)(nil)
var _ admin.AIAdminServer = (*AIOpsPlugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()

	p := NewPlugin(ctx)

	p.storageNamespace = os.Getenv("POD_NAMESPACE")

	go p.setOpensearchConnection()

	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(
		util.PackService(&modeltraining.ModelTraining_ServiceDesc, p),
		util.PackService(&admin.AIAdmin_ServiceDesc, p),
	))
	return scheme
}
