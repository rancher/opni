package slo

import (
	"context"

	"log/slog"

	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/apis/slo"

	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	managementext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/management"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
)

type Plugin struct {
	slo.UnsafeSLOServer
	system.UnimplementedSystemPluginClient

	ctx    context.Context
	logger *slog.Logger

	storage             future.Future[StorageAPIs]
	mgmtClient          future.Future[managementv1.ManagementClient]
	adminClient         future.Future[cortexadmin.CortexAdminClient]
	alertEndpointClient future.Future[alertingv1.AlertEndpointsClient]
}

// ManagementServices implements managementext.ManagementAPIExtension.
func (p *Plugin) ManagementServices(_ managementext.ServiceController) []util.ServicePackInterface {
	return []util.ServicePackInterface{
		util.PackService[slo.SLOServer](&slo.SLO_ServiceDesc, p),
	}
}

// Authorized checks whether a given set of roles is allowed to access a given request
func (p *Plugin) CheckAuthz(_ context.Context, _ *corev1.ReferenceList, _, _ string) bool {
	return true
}

type StorageAPIs struct {
	SLOs     storage.KeyValueStoreT[*slo.SLOData]
	Services storage.KeyValueStoreT[*slo.Service]
	Metrics  storage.KeyValueStoreT[*slo.Metric]
}

func NewPlugin(ctx context.Context) *Plugin {
	return &Plugin{
		ctx:                 ctx,
		logger:              logger.NewPluginLogger().WithGroup("slo"),
		storage:             future.New[StorageAPIs](),
		mgmtClient:          future.New[managementv1.ManagementClient](),
		adminClient:         future.New[cortexadmin.CortexAdminClient](),
		alertEndpointClient: future.New[alertingv1.AlertEndpointsClient](),
	}
}

var _ slo.SLOServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	scheme.Add(managementext.ManagementAPIExtensionPluginID, managementext.NewPlugin(p))
	return scheme
}
