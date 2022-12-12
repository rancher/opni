package management

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kralicky/grpc-gateway/v2/runtime"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/plugins/meta"
	"github.com/rancher/opni/pkg/plugins/types"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/emptypb"
)

type statusHTTPDesc struct {
	client types.BackendStatusPlugin
	path   string
}

func (s *Server) configureStatusEndpoints(ctx context.Context, pl plugins.LoaderInterface) {
	pl.Hook(hooks.OnLoadM(func(p types.BackendStatusPlugin, md meta.PluginMeta) {
		s.statusMu.Lock()
		s.statusDescs = append(s.statusDescs, statusHTTPDesc{
			client: p,
			path:   fmt.Sprintf("/%s/status", md.ShortName()),
		})
		s.statusMu.Unlock()
	}))
}

func (s *Server) configureStatusHTTPHandler(mux *runtime.ServeMux) {
	lg := s.logger
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	for _, desc := range s.statusDescs {
		if err := mux.HandlePath(http.MethodGet, desc.path, statusHandler(desc, mux)); err != nil {
			lg.With(
				zap.Error(err),
				zap.String("path", desc.path),
			).Error("failed to configure http handler")
		} else {
			lg.With(
				zap.String("path", desc.path),
			).Debug("configured http handler")
		}
	}
}

func statusHandler(desc statusHTTPDesc, mux *runtime.ServeMux) runtime.HandlerFunc {
	lg := logger.New().Named("statushandler")
	return func(w http.ResponseWriter, req *http.Request, pathParams map[string]string) {
		lg := lg.With(
			"path", desc.path,
		)

		lg.Debug("handling http request")
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		_, outboundMarshaler := runtime.MarshalerForRequest(mux, req)

		resp, err := desc.client.PluginStatus(ctx, &emptypb.Empty{})
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}

		jsonResp, err := json.Marshal(resp)
		if err != nil {
			lg.Error(err)
			runtime.HTTPError(ctx, mux, outboundMarshaler, w, req, err)
			return
		}

		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		if _, err := w.Write(jsonResp); err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to write response")
		}
	}
}
