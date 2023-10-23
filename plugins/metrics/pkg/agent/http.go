package agent

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/util/push"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/plugins/metrics/apis/node"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log/slog"

	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
)

type HttpServer struct {
	apiextensions.UnsafeHTTPAPIExtensionServer

	logger *slog.Logger

	remoteWriteClientMu sync.RWMutex
	remoteWriteClient   clients.Locker[remotewrite.RemoteWriteClient]

	conditions health.ConditionTracker

	enabled atomic.Bool
}

func NewHttpServer(ct health.ConditionTracker, lg *slog.Logger) *HttpServer {
	return &HttpServer{
		logger:     lg,
		conditions: ct,
	}
}

func (s *HttpServer) SetEnabled(enabled bool) {
	if enabled {
		s.conditions.Set(node.CondRemoteWrite, health.StatusPending, "")
	} else {
		s.conditions.Clear(node.CondRemoteWrite)
	}
	s.enabled.Store(enabled)
}

func (s *HttpServer) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	s.remoteWriteClientMu.Lock()
	defer s.remoteWriteClientMu.Unlock()

	s.remoteWriteClient = client
}

func (s *HttpServer) ConfigureRoutes(router *gin.Engine) {
	router.POST("/api/agent/push", gin.WrapH(push.Handler(100<<20, nil, s.pushFunc)))
	pprof.Register(router, "/debug/plugin_metrics/pprof")
}

func (s *HttpServer) pushFunc(ctx context.Context, writeReq *cortexpb.WriteRequest) (writeResp *cortexpb.WriteResponse, writeErr error) {
	if !s.enabled.Load() {
		return nil, status.Errorf(codes.Unavailable, "api not enabled")
	}
	s.remoteWriteClientMu.RLock()
	defer s.remoteWriteClientMu.RUnlock()
	if s.remoteWriteClient == nil {
		return nil, status.Errorf(codes.Unavailable, "gateway not connected")
	}

	ok := s.remoteWriteClient.Use(func(rwc remotewrite.RemoteWriteClient) {
		if rwc == nil {
			s.conditions.Set(node.CondRemoteWrite, health.StatusPending, "gateway not connected")
			return
		}
		writeResp, writeErr = rwc.Push(ctx, writeReq)
	})
	if !ok {
		return nil, status.Errorf(codes.Unavailable, "gateway not connected")
	}
	return
}
