package backend

import (
	"context"

	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
)

type ServiceBackend interface {
	ListServices(ctx context.Context, req *slov1.ListServicesRequest) (*slov1.ServiceList, error)
	ListMetrics(ctx context.Context, req *slov1.ListMetricsRequest) (*slov1.MetricGroupList, error)
	ListEvents(ctx context.Context, req *slov1.ListEventsRequest) (*slov1.EventList, error)
}
