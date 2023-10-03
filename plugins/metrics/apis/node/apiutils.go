package node

import driverutil "github.com/rancher/opni/pkg/plugins/driverutil"

type SpecializedConfigServer interface {
	driverutil.BasicDefaultServer[
		*MetricsCapabilityConfig,
		*driverutil.GetRequest,
		*SetRequest,
	]
	driverutil.BasicActiveServer[
		*MetricsCapabilityConfig,
		*NodeGetRequest,
		*NodeSetRequest,
	]
	driverutil.ResetServer[
		*MetricsCapabilityConfig,
		*NodeResetRequest,
	]
	driverutil.HistoryServer[
		*MetricsCapabilityConfig,
		*ConfigurationHistoryRequest,
		*ConfigurationHistoryResponse,
	]
}
