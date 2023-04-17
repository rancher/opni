package agent

import (
	"context"
	"sync"

	"sync/atomic"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/cortexproject/cortex/pkg/util/push"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	httputil "github.com/rancher/opni/pkg/util/http"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
)

type HttpServer struct {
	apiextensions.UnsafeHTTPAPIExtensionServer

	handlers []httputil.GinHandler
	logger   *zap.SugaredLogger

	remoteWriteClientMu sync.RWMutex
	remoteWriteClient   clients.Locker[remotewrite.RemoteWriteClient]

	conditions health.ConditionTracker

	enabled atomic.Bool
}

func NewHttpServer(ct health.ConditionTracker, lg *zap.SugaredLogger, handlers []httputil.GinHandler) *HttpServer {
	return &HttpServer{
		logger:     lg,
		conditions: ct,
		handlers:   handlers,
	}
}

func (s *HttpServer) SetEnabled(enabled bool) {
	if enabled {
		s.conditions.Set(CondRemoteWrite, health.StatusPending, "")
	} else {
		s.conditions.Clear(CondRemoteWrite)
	}
	s.enabled.Store(enabled)
}

func (s *HttpServer) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	s.remoteWriteClientMu.Lock()
	defer s.remoteWriteClientMu.Unlock()

	s.remoteWriteClient = client
}

func (s *HttpServer) ConfigureRoutes(router *gin.Engine) {
	httputil.RegisterAdditionalHandlers(
		router,
		s.handlers,
	)
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
			s.conditions.Set(CondRemoteWrite, health.StatusPending, "gateway not connected")
			return
		}
		writeResp, writeErr = rwc.Push(ctx, writeReq)
	})
	if !ok {
		return nil, status.Errorf(codes.Unavailable, "gateway not connected")
	}
	return
}
