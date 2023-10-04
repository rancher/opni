package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/alerting/server"
	ssync "github.com/rancher/opni/pkg/alerting/server/sync"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/util"
	"go.uber.org/zap"
)

const proxyPath = "/plugin_alerting/alertmanager"

type ProxyServer struct {
	util.Initializer
	lg *zap.SugaredLogger

	proxy *alertmanagerProxy
}

var _ server.ServerComponent = (*ProxyServer)(nil)

func (p *ProxyServer) Initialize() {
}

func (p *ProxyServer) Name() string {
	return "alerting-proxy"
}

func (p *ProxyServer) Healthy() bool {
	return p.Initialized()
}

func (p *ProxyServer) Ready() bool {
	return p.Initialized()
}

func (p *ProxyServer) Sync(_ context.Context, _ ssync.SyncInfo) error {
	return nil
}

func (p *ProxyServer) Status() server.Status {
	return server.Status{
		Running: p.Ready() && p.Healthy(),
	}
}

func (p *ProxyServer) SetConfig(cfg server.Config) {
	p.proxy.SetConfig(cfg)
}

func NewProxyServer(
	lg *zap.SugaredLogger,
) *ProxyServer {
	return &ProxyServer{
		lg:    lg,
		proxy: newAlertmanagerProxy(lg),
	}
}

func (p *ProxyServer) ConfigureRoutes(router *gin.Engine) {
	router.Any(
		fmt.Sprintf("%s/*any", proxyPath),
		gin.WrapH(
			http.StripPrefix(
				proxyPath,
				p.proxy,
			),
		),
	)
}

var _ httpext.HTTPAPIExtension = (*ProxyServer)(nil)

type alertmanagerProxy struct {
	lg           *zap.SugaredLogger
	configMu     sync.RWMutex
	reverseProxy *httputil.ReverseProxy
}

func (a *alertmanagerProxy) SetConfig(config server.Config) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if config.Client == nil {
		a.lg.Info("disabling alertmanager proxy...")
		a.reverseProxy = nil
		return
	}
	targetURL := config.Client.ProxyClient().ProxyURL()
	a.lg.Infof("configuring alertmanager proxy to : %s", targetURL.String())
	a.reverseProxy = httputil.NewSingleHostReverseProxy(targetURL)
}

func newAlertmanagerProxy(lg *zap.SugaredLogger) *alertmanagerProxy {
	return &alertmanagerProxy{
		reverseProxy: nil,
		lg:           lg,
	}
}

func (a *alertmanagerProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	if a.reverseProxy == nil {
		http.Error(w, "Alertmanager proxy unavailable", http.StatusServiceUnavailable)
		return
	}
	r.URL.Scheme = "http"
	a.reverseProxy.ServeHTTP(w, r)
}
