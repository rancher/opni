package alerting

import (
	"context"

	"github.com/hashicorp/go-hclog"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/util/future"
	apis "github.com/rancher/opni/plugins/alerting/pkg/apis/alerting"
)

type Plugin struct {
	apis.UnsafeAlertingServer
	system.UnimplementedSystemPluginClient
	ctx        context.Context
	logger     hclog.Logger
	mgmtClient future.Future[managementv1.ManagementClient]
}

type StorageAPIs struct {
}

func NewPlugin(ctx context.Context) *Plugin {
	lg := logger.NewForPlugin()
	lg.SetLevel(hclog.Debug)
	return &Plugin{
		ctx:        ctx,
		logger:     lg,
		mgmtClient: future.New[managementv1.ManagementClient](),
	}
}

var _ apis.AlertingServer = (*Plugin)(nil)

func Scheme(ctx context.Context) meta.Scheme {
	scheme := meta.NewScheme()
	p := NewPlugin(ctx)
	scheme.Add(system.SystemPluginID, system.NewPlugin(p))
	return scheme
}
