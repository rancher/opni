package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/alerting/server"
	ssync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/pkg/logger"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
)

const proxyPath = "/plugin_alerting/alertmanager"

type ProxyServer struct {
	ctx context.Context

	*alertmanagerProxy
}

var _ server.ServerComponent = (*ProxyServer)(nil)

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
	p.alertmanagerProxy.SetConfig(cfg)
}

func NewProxyServer(
	ctx context.Context,
) *ProxyServer {
	return &ProxyServer{
		ctx:               ctx,
		alertmanagerProxy: newAlertmanagerProxy(ctx),
	}
}

func (p *ProxyServer) ConfigureRoutes(router *gin.Engine) {
	router.Any(
		fmt.Sprintf("%s/*any", proxyPath),
		gin.WrapH(
			http.StripPrefix(
				proxyPath,
				p.alertmanagerProxy,
			),
		),
	)
}

var _ httpext.HTTPAPIExtension = (*ProxyServer)(nil)

type alertmanagerProxy struct {
	util.Initializer

	ctx context.Context

	tlsConfig    future.Future[*tls.Config]
	configMu     sync.RWMutex
	reverseProxy *httputil.ReverseProxy
}

func (a *alertmanagerProxy) Initialize(tlsConfig *tls.Config) {
	a.InitOnce(func() {
		a.tlsConfig.Set(tlsConfig)
	})
}

func (a *alertmanagerProxy) SetConfig(config server.Config) {
	lg := logger.PluginLoggerFromContext(a.ctx)
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if config.Client == nil {
		lg.Info("disabling alertmanager proxy...")
		a.reverseProxy = nil
		return
	}
	targetURL := config.Client.ProxyClient().ProxyURL()
	lg.Info(fmt.Sprintf("configuring alertmanager proxy to : %s", targetURL.String()))
	ctxca, ca := context.WithTimeout(context.Background(), time.Second)
	defer ca()
	tlsConfig, err := a.tlsConfig.GetContext(ctxca)
	if err != nil {
		lg.Error("tls config for alertmanager reverse proxy is not initialized")
		a.reverseProxy = nil
		return
	}
	httptransport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	reverseProxy := httputil.NewSingleHostReverseProxy(targetURL)
	reverseProxy.Transport = httptransport
	a.reverseProxy = reverseProxy
}

func newAlertmanagerProxy(ctx context.Context) *alertmanagerProxy {
	return &alertmanagerProxy{
		ctx:          ctx,
		reverseProxy: nil,
		tlsConfig:    future.New[*tls.Config](),
	}
}

func (a *alertmanagerProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	if a.reverseProxy == nil {
		http.Error(w, "Alertmanager proxy unavailable", http.StatusServiceUnavailable)
		return
	}
	r.Close = true
	a.reverseProxy.ServeHTTP(w, r)
}
