package alerting

import (
	"context"

	"github.com/hashicorp/go-hclog"
	"github.com/rancher/opni/pkg/alerting"
	"github.com/rancher/opni/pkg/storage"

	lru "github.com/hashicorp/golang-lru"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
)

const AlertingLogCacheSize = 32

type Plugin struct {
	alertingv1alpha.UnsafeAlertingServer
	system.UnimplementedSystemPluginClient
	ctx             context.Context
	logger          hclog.Logger
	inMemCache      *lru.Cache
	endpointBackend future.Future[RuntimeEndpointBackend]
	alertingOptions future.Future[AlertingOptions]
	storage         future.Future[StorageAPIs]
	mgmtClient      future.Future[managementv1.ManagementClient]
}

type AlertingOptions struct {
	Endpoints   []string
	ConfigMap   string
	Namespace   string
	StatefulSet string
}

type StorageAPIs struct {
	Conditions    storage.KeyValueStoreT[*alertingv1alpha.AlertCondition]
	AlertEndpoint storage.KeyValueStoreT[*alertingv1alpha.AlertEndpoint]
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewForPlugin()
	lg.SetLevel(hclog.Info)
	return &Plugin{
		ctx:             ctx,
		logger:          lg,
		inMemCache:      nil,
		mgmtClient:      future.New[managementv1.ManagementClient](),
		endpointBackend: future.New[RuntimeEndpointBackend](),
		alertingOptions: future.New[AlertingOptions](),
		storage:         future.New[StorageAPIs](),
	}
}

var _ alertingv1alpha.AlertingServer = (*Plugin)(nil)
var _ alerting.Provider = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID,
		managementext.NewPlugin(&alertingv1alpha.Alerting_ServiceDesc, p))
	return scheme
}
