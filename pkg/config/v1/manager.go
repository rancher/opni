package v1

import (
	context "context"

	driverutil "github.com/rancher/opni/pkg/plugins/driverutil"
)

type GatewayConfigManager struct {
	*driverutil.BaseConfigServer[
		*driverutil.GetRequest,
		*SetRequest,
		*ResetRequest,
		*driverutil.ConfigurationHistoryRequest,
		*HistoryResponse,
		*GatewayConfigSpec,
	]
}

func (s *GatewayConfigManager) DryRun(ctx context.Context, req *DryRunRequest) (*DryRunResponse, error) {
	res, err := s.Tracker().DryRun(ctx, req)
	if err != nil {
		return nil, err
	}
	return &DryRunResponse{
		Current:          res.Current,
		Modified:         res.Modified,
		ValidationErrors: res.ValidationErrors.ToProto(),
	}, nil
}
