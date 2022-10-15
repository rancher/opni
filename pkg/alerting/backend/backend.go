package backend

import (
	"context"
	"fmt"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	"go.uber.org/zap"
	"path"
	"strings"
)

const (
	GET    = "GET"
	POST   = "POST"
	DELETE = "DELETE"
	v2     = "/api/v2"
	v1     = "/api/v1"
)

type RuntimeEndpointBackend interface {
	Fetch(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string) (string, error)
	Put(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string, data *routing.RoutingTree) error
	Reload(ctx context.Context, lg *zap.SugaredLogger, options shared.NewAlertingOptions, key string) error
	Port() int
}

type AlertManagerAPI struct {
	Endpoint string
	Api      string
	Route    string
	Verb     string
}

func (a *AlertManagerAPI) Construct() string {
	return a.Endpoint + path.Join(a.Api, a.Route)
}

func (a *AlertManagerAPI) ConstructHTTP() string {
	tempRes := a.Construct()
	if !strings.HasPrefix(tempRes, "http://") {
		return fmt.Sprintf("http://%s", tempRes)
	}
	return tempRes
}

func (a *AlertManagerAPI) ConstructHTTPS() string {
	tempRes := a.Construct()
	if !strings.HasPrefix("http://", tempRes) {
		return fmt.Sprintf("https://%s", tempRes)
	} else if strings.HasPrefix("http://", tempRes) {
		return strings.Replace(tempRes, "http://", "https://", 1)
	} else {
		return tempRes
	}
}

func (a *AlertManagerAPI) IsReady() bool {
	return false
}

func (a *AlertManagerAPI) IsHealthy() bool {
	return false
}

// WithHttpV2
// ## OpenAPI reference
// https://github.com/prometheus/alertmanager/blob/main/api/v2/openapi.yaml
func (a *AlertManagerAPI) WithHttpV2() *AlertManagerAPI {
	a.Api = v2
	return a
}

// WithHttpV1
// ## Reference
// https://prometheus.io/docs/alerting/latest/clients/
func (a *AlertManagerAPI) WithHttpV1() *AlertManagerAPI {
	a.Api = v1
	return a
}
