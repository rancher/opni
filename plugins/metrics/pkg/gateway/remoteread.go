package gateway

import (
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"go.uber.org/zap"
)

type RemoteReadConfig struct {
	Config *v1beta1.GatewayConfigSpec `validate:"required"`
	Logger *zap.SugaredLogger         `validate:"required"`
}

type RemoteReader struct {
	remoteread.UnsafeRemoteReadServer
	RemoteReadConfig

	util.Initializer
}
