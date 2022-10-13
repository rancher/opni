package agent

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"sync/atomic"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/health"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
)

type HttpServer struct {
	apiextensions.UnsafeHTTPAPIExtensionServer

	logger *zap.SugaredLogger

	remoteWriteClientMu sync.RWMutex
	remoteWriteClient   clients.Locker[remotewrite.RemoteWriteClient]

	conditions health.ConditionTracker

	enabled atomic.Bool
}

func NewHttpServer(ct health.ConditionTracker, lg *zap.SugaredLogger) *HttpServer {
	return &HttpServer{
		logger:     lg,
		conditions: ct,
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
	router.POST("/api/agent/push", s.handlePushRequest)
}

func (s *HttpServer) handlePushRequest(c *gin.Context) {
	if !s.enabled.Load() {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	s.remoteWriteClientMu.RLock()
	defer s.remoteWriteClientMu.RUnlock()
	if s.remoteWriteClient == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	ok := s.remoteWriteClient.Use(func(rwc remotewrite.RemoteWriteClient) {
		if rwc == nil {
			s.conditions.Set(CondRemoteWrite, health.StatusPending, "gateway not connected")
			c.Error(errors.New("gateway not connected"))
			c.String(http.StatusServiceUnavailable, "gateway not connected")
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		_, err = rwc.Push(c.Request.Context(), &remotewrite.Payload{
			Contents: body,
		})
		var respCode int
		if err != nil {
			stat := status.Convert(err)
			// check if statusCode is a valid HTTP status code
			if stat.Code() >= 100 && stat.Code() <= 599 {
				respCode = int(stat.Code())
			} else {
				respCode = http.StatusServiceUnavailable
			}
			// As a special case, status code 400 may indicate a success.
			// Cortex handles a variety of cases where prometheus would normally
			// return an error, such as duplicate or out of order samples. Cortex
			// will return code 400 to prometheus, which prometheus will treat as
			// a non-retriable error. In this case, the remote write status condition
			// will be cleared as if the request succeeded.
			message := stat.Message()
			if respCode == http.StatusBadRequest {
				if strings.Contains(message, "out of bounds") ||
					strings.Contains(message, "out of order sample") ||
					strings.Contains(message, "duplicate sample for timestamp") ||
					strings.Contains(message, "exemplars not ingested because series not already present") {
					{
						s.conditions.Clear(CondRemoteWrite)
						c.Error(errors.New("soft error (request succeeded): " + message))
						respCode = http.StatusOK // try returning 200, prometheus may be throttling on 400
					}
				}
			} else {
				s.conditions.Set(CondRemoteWrite, health.StatusFailure, stat.Message())
				c.Error(err)
			}

			c.String(respCode, message)
			return
		}
		s.conditions.Clear(CondRemoteWrite)
		c.Status(http.StatusOK)
	})

	if !ok {
		c.Status(http.StatusServiceUnavailable)
	}
}
