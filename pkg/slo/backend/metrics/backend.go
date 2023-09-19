package metrics

import (
	"context"
	"fmt"

	slov1 "github.com/rancher/opni/pkg/apis/slo/v1"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/slo/backend"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/tidwall/gjson"
)

type MetricsBackend struct {
	*MetricsBackendProvider
}

func NewBackend(p *MetricsBackendProvider) *MetricsBackend {
	return &MetricsBackend{
		MetricsBackendProvider: p,
	}
}

var _ backend.ServiceBackend = &MetricsBackend{}

func (m *MetricsBackend) ListServices(ctx context.Context, req *slov1.ListServicesRequest) (*slov1.ServiceList, error) {
	services := &slov1.ServiceList{}
	// we should move this to a more generic label discovery API
	discoveryQuery := `group by (job) ({__name__!=""})`
	resp, err := m.adminClient.Get().Query(
		ctx,
		&cortexadmin.QueryRequest{
			Tenants: []string{req.GetClusterId()},
			Query:   discoveryQuery,
		})
	if err != nil {
		return nil, err
	}

	qr, err := compat.UnmarshalPrometheusResponse(resp.Data)
	if err != nil {
		return nil, err
	}

	vec, err := qr.GetVector()
	if err != nil {
		return nil, err
	}
	if vec == nil {
		return nil, fmt.Errorf("could not unmarshal cortex query response to json : expeted model.Vector")
	}
	for _, vecSample := range *vec {
		metric := vecSample.Metric
		job, ok := metric["job"]
		if !ok {
			continue
		}
		services.Items = append(services.Items, &slov1.Service{
			ClusterId: req.GetClusterId(),
			ServiceId: string(job),
		})
	}

	result := gjson.Get(string(resp.Data), "data.result.#.metric.job")
	if !result.Exists() {
		return nil, fmt.Errorf("could not unmarshal cortex query response to expected json")
	}
	for _, v := range result.Array() {
		services.Items = append(services.Items, &slov1.Service{
			ClusterId: req.GetClusterId(),
			ServiceId: v.String(),
		})
	}
	return services, nil
}
func (m *MetricsBackend) ListMetrics(ctx context.Context, req *slov1.ListMetricsRequest) (*slov1.MetricGroupList, error) {
	resp, err := m.adminClient.Get().GetSeriesMetrics(
		ctx,
		&cortexadmin.SeriesRequest{
			Tenant: req.GetClusterId(),
			JobId:  req.GetServiceId(),
		},
	)
	if err != nil {
		return nil, err
	}
	return scoredLabels(resp, m.filters), nil
}

func (m *MetricsBackend) ListEvents(ctx context.Context, req *slov1.ListEventsRequest) (*slov1.EventList, error) {
	events := &slov1.EventList{
		Items: []*slov1.Event{},
	}

	resp, err := m.adminClient.Get().GetMetricLabelSets(
		ctx,
		&cortexadmin.LabelRequest{
			Tenant:     req.GetClusterId(),
			JobId:      req.GetServiceId(),
			MetricName: req.GetMetricId(),
		},
	)
	if err != nil {
		return nil, err
	}
	for _, item := range resp.GetItems() {
		events.Items = append(events.Items, &slov1.Event{
			Key:  item.GetName(),
			Vals: item.GetItems(),
		})
	}

	return events, nil
}
