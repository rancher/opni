package node

import driverutil "github.com/rancher/opni/pkg/plugins/driverutil"

type SpecializedConfigServer interface {
	driverutil.ConfigServer[
		*MetricsCapabilityConfig,
		*GetRequest,
		*SetRequest,
		*ResetRequest,
		*ConfigurationHistoryRequest,
		*ConfigurationHistoryResponse,
	]
}
