package gateway

import (
	"context"
	"fmt"
	"net"

	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/util"
)

type PprofEndpointHandler struct {
	cfg v1beta1.ProfilingSpec
}

func NewPprofEndpointHandler(cfg v1beta1.ProfilingSpec) *PprofEndpointHandler {
	return &PprofEndpointHandler{
		cfg: cfg,
	}
}

func (h *PprofEndpointHandler) ListenAndServe(ctx context.Context) error {
	listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", h.cfg.Port))
	if err != nil {
		return err
	}

	router := gin.New()
	pprof.Register(router)

	return util.ServeHandler(ctx, router.Handler(), listener)
}
