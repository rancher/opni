package management

import (
	"context"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Server) GetAgentLogs(ctx context.Context, req *managementv1.StreamAgentLogsRequest) (*controlv1.StructuredLogRecords, error) {
	if m.agentControlDataSource == nil {
		return nil, status.Error(codes.Unavailable, "agent control API not configured")
	}

	if err := validation.Validate(req); err != nil {
		return nil, err
	}

	logs, err := m.agentControlDataSource.GetLogs(ctx, req.Agent, req.Request)
	if err != nil {
		m.logger.Error("error requesting agent logs", logger.Err(err))
		return nil, err
	}

	return logs, nil
}
