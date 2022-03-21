package webui

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/logger"
	"github.com/rancher/opni-monitoring/web"
	"github.com/vearutop/statigz"
	"github.com/vearutop/statigz/brotli"
	"go.uber.org/zap"
)

type ExtraHandler struct {
	Path    string
	Handler http.HandlerFunc
}

var (
	ExtraHandlers = []ExtraHandler{}
)

func AddExtraHandler(path string, handler http.HandlerFunc) {
	ExtraHandlers = append(ExtraHandlers, ExtraHandler{
		Path:    path,
		Handler: handler,
	})
}

type WebUIServer struct {
	config *v1beta1.GatewayConfig
	server *http.Server
	mu     sync.Mutex
	logger *zap.SugaredLogger
}

func NewWebUIServer(config *v1beta1.GatewayConfig) (*WebUIServer, error) {
	if !web.EmbeddedAssetsAvailable() {
		return nil, errors.New("embedded assets not available")
	}
	if config.Spec.Management.WebListenAddress == "" {
		return nil, errors.New("management.webListenAddress not set in config")
	}
	return &WebUIServer{
		config: config,
		logger: logger.New().Named("webui"),
	}, nil
}

func (ws *WebUIServer) ListenAndServe() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	lg := ws.logger
	listener, err := net.Listen("tcp4", ws.config.Spec.Management.WebListenAddress)
	if err != nil {
		return err
	}
	lg.With(
		"address", listener.Addr(),
	).Info("ui server starting")

	mux := http.NewServeMux()
	ws.server = &http.Server{
		Handler: mux,
	}

	// 200.html (app entrypoint)
	twoHundred, err := web.DistFS.ReadFile("dist/200.html")
	if err != nil {
		return err
	}

	// Static assets
	sub, err := fs.Sub(web.DistFS, "dist")
	if err != nil {
		return err
	}
	brotliSrv := statigz.FileServer(sub.(fs.ReadDirFS), brotli.AddEncoding)
	fileSrv := http.FileServer(http.FS(sub))
	mux.Handle("/_nuxt/", brotliSrv)
	mux.Handle("/.nojekyll", fileSrv)
	mux.Handle("/favicon.ico", fileSrv)
	mux.Handle("/favicon.png", fileSrv)
	mux.Handle("/loading-indicator.html", fileSrv)

	// Fake out Steve and Norman
	mux.HandleFunc("/v1/", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
	})
	mux.HandleFunc("/v3/", func(rw http.ResponseWriter, r *http.Request) {
		rw.WriteHeader(200)
	})

	opniApiAddr := ws.config.Spec.Management.HTTPListenAddress
	mgmtUrl, err := url.Parse("http://" + opniApiAddr)
	if err != nil {
		lg.With(
			"url", opniApiAddr,
			"error", err,
		).Fatal("failed to parse management API URL")
		return err
	}
	mux.HandleFunc("/opni-api/", func(rw http.ResponseWriter, r *http.Request) {
		// round-trip to the management API
		// strip the prefix /opni-api/
		u := *mgmtUrl
		u.Path = r.URL.Path[len("/opni-api/"):]

		req, err := http.NewRequest(r.Method, u.String(), r.Body)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to create request")
			rw.WriteHeader(500)
			return
		}
		req.Header = r.Header
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lg.With(
				zap.Error(err),
			).Error("failed to round-trip management api request")
			rw.WriteHeader(500)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			for _, vv := range v {
				rw.Header().Add(k, vv)
			}
		}
		rw.WriteHeader(resp.StatusCode)
		io.Copy(rw, resp.Body)
	})
	for _, h := range ExtraHandlers {
		lg.With(zap.String("path", h.Path)).Debug("adding extra handler")
		mux.HandleFunc(h.Path, h.Handler)
	}
	mux.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/html")
		rw.WriteHeader(200)
		// serve 200.html
		rw.Write(twoHundred)
	})
	return ws.server.Serve(listener)
}

func (ws *WebUIServer) Shutdown(ctx context.Context) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.server == nil {
		return nil
	}
	return ws.server.Shutdown(ctx)
}
