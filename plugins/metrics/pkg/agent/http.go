package agent

import (
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"google.golang.org/grpc/status"

	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/plugins/apis/apiextensions"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
)

type HttpServer struct {
	apiextensions.UnsafeHTTPAPIExtensionServer

	logger *zap.SugaredLogger

	remoteWriteClientMu sync.RWMutex
	remoteWriteClient   clients.Locker[remotewrite.RemoteWriteClient]
}

func NewHttpServer(lg *zap.SugaredLogger) *HttpServer {
	return &HttpServer{
		logger: lg,
	}
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
	var code int
	s.remoteWriteClientMu.RLock()
	defer s.remoteWriteClientMu.RUnlock()
	if s.remoteWriteClient == nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	ok := s.remoteWriteClient.Use(func(rwc remotewrite.RemoteWriteClient) {
		if rwc == nil {
			// s.setCondition(condRemoteWrite, statusPending, "gateway not connected")
			code = http.StatusServiceUnavailable
			return
		}
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.Error(err)
			code = http.StatusBadRequest
			return
		}
		_, err = rwc.Push(c.Request.Context(), &remotewrite.Payload{
			// AuthorizedClusterID: s.tenantID,
			Contents: body,
		})
		if err != nil {
			stat := status.Convert(err)
			// check if statusCode is a valid HTTP status code
			if stat.Code() >= 100 && stat.Code() <= 599 {
				code = int(stat.Code())
			} else {
				code = http.StatusServiceUnavailable
			}
			// As a special case, status code 400 may indicate a success.
			// Cortex handles a variety of cases where prometheus would normally
			// return an error, such as duplicate or out of order samples. Cortex
			// will return code 400 to prometheus, which prometheus will treat as
			// a non-retriable error. In this case, the remote write status condition
			// will be cleared as if the request succeeded.
			if code == http.StatusBadRequest {
				// s.clearCondition(condRemoteWrite)
				c.Error(errors.New("soft error: this request likely succeeded"))
			} else {
				// s.setCondition(condRemoteWrite, statusFailure, stat.Message())
			}
			c.Error(err)
			return
		}
		// s.clearCondition(condRemoteWrite)
		code = http.StatusOK
	})
	if !ok {
		code = http.StatusServiceUnavailable
	}
	c.Status(code)
}
