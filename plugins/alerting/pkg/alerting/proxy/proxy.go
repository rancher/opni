package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rancher/opni/pkg/alerting/server"
	ssync "github.com/rancher/opni/pkg/alerting/server/sync"
	httpext "github.com/rancher/opni/pkg/plugins/apis/apiextensions/http"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"go.uber.org/zap"
)

const proxyPath = "/plugin_alerting/alertmanager"

type ProxyServer struct {
	lg *zap.SugaredLogger

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
	lg *slog.Logger,
) *ProxyServer {
	return &ProxyServer{
		lg:                lg,
		alertmanagerProxy: newAlertmanagerProxy(lg),
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

	lg *zap.SugaredLogger

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
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if config.Client == nil {
		a.lg.Info("disabling alertmanager proxy...")
		a.reverseProxy = nil
		return
	}
	targetURL := config.Client.ProxyClient().ProxyURL()
	a.lg.Infof("configuring alertmanager proxy to : %s", targetURL.String())
	ctxca, ca := context.WithTimeout(context.Background(), time.Second)
	defer ca()
	tlsConfig, err := a.tlsConfig.GetContext(ctxca)
	if err != nil {
		a.lg.Error("tls config for alertmanager reverse proxy is not initialized")
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

func newAlertmanagerProxy(lg *slog.Logger) *alertmanagerProxy {
	return &alertmanagerProxy{
		reverseProxy: nil,
		lg:           lg,
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
	r.URL.Scheme = "http"
	a.reverseProxy.ServeHTTP(w, r)
}
