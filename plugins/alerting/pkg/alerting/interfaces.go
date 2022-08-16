package alerting

import (
	"context"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"sync"
)

var mu sync.Mutex
var datasourceToAlertingCapability map[string]AlertingStore = make(map[string]AlertingStore)

func RegisterDatasource(datasource string, store AlertingStore) {
	defer mu.Unlock()
	mu.Lock()
	datasourceToAlertingCapability[datasource] = store
}

func UnregisterDatasource(datasource string) {
	defer mu.Unlock()
	mu.Lock()
	delete(datasourceToAlertingCapability, datasource)
}

type AlertingStore interface {
}

type RequestBase struct {
	req proto.Message
	p   *Plugin
	ctx context.Context
	lg  *zap.SugaredLogger
}

type AlertingMonitoring struct {
	RequestBase
}

func NewAlertingMonitoringStore(p *Plugin, lg *zap.SugaredLogger) *AlertingMonitoring {
	return &AlertingMonitoring{
		RequestBase{
			req: nil,
			p:   p,
			ctx: context.Background(),
			lg:  lg,
		},
	}
}
