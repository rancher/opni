package management

import (
	"io"

	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (m *Server) GetAgentLogStream(req *managementv1.StreamAgentLogsRequest, server managementv1.Management_GetAgentLogStreamServer) error {
	if m.agentControlDataSource == nil {
		return status.Error(codes.Unavailable, "agent control API not configured")
	}

	if err := validation.Validate(req); err != nil {
		return err
	}

	logStream, err := m.agentControlDataSource.StreamLogs(server.Context(), req.Agent, req.Request)
	if err != nil || logStream == nil {
		m.logger.Error("error streaming agent logs", logger.Err(err))
		return err
	}

	for {
		log, err := logStream.Recv()
		done := err == io.EOF
		keepFollowing := done && req.Request.Follow
		if keepFollowing {
			continue
		} else if done {
			return nil
		} else if err != nil {
			return err
		}

		err = server.Send(log)
		if err != nil {
			return err
		}
	}

}
