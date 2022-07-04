package slo

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/pkg/slo/query"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type metricTemplate struct {
	NameRegex string
	ServiceId string
}

func doDownstreamQuery(p *Plugin, ctx context.Context, metricRequest *api.MetricRequest, q query.Query) (string, error) {
	lg := p.logger
	var queryBytes bytes.Buffer
	if err := query.GetDownstreamMetricQueryTempl.Execute(&queryBytes,
		metricTemplate{
			NameRegex: q.GetMetricFilter(),
			ServiceId: metricRequest.ServiceId,
		},
	); err != nil {
		return "", err
	}
	resp, err := p.adminClient.Get().Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{metricRequest.ClusterId},
		Query:   strings.TrimSpace(queryBytes.String()),
	})
	if err != nil {
		lg.Error(fmt.Sprintf("Failed to query cluster %v: %v", metricRequest.ClusterId, err))
		return "", err
	}
	res := make([]string, 0)
	data := resp.GetData()
	lg.Debug(fmt.Sprintf("Received service data %s from cluster %s ", string(data), metricRequest.ClusterId))
	qq, err := unmarshal.UnmarshallPrometheusResponse(data)
	if err != nil {
		return "", err
	}
	switch qq.V.Type() {
	case model.ValVector:
		vv := qq.V.(model.Vector)
		if len(vv) == 0 {
			err := status.Error(codes.NotFound,
				fmt.Sprintf("No assignable metric '%s' for service '%s' in cluster '%s' ",
					metricRequest.Name, metricRequest.ServiceId, metricRequest.ClusterId))
			return "", err
		}
		for _, v := range vv {
			res = append(res, string(v.Metric["__name__"]))
		}
		// should always have one + metric
		lg.Debug(fmt.Sprintf("Found metricIds : %v", res))
		metricId := q.BestMatch(res)
		if metricId == "" {
			err := status.Error(codes.NotFound,
				fmt.Sprintf("No assignable metric '%s' for service '%s' in cluster '%s' ",
					metricRequest.Name, metricRequest.ServiceId, metricRequest.ClusterId))
			return "", err
		}
		return metricId, nil
	}
	return "", fmt.Errorf("Could not unmarshall response into expected format")
}

func assignMetricToJobId(p *Plugin, ctx context.Context, metricRequest *api.MetricRequest) (string, string, error) {
	queryStruct := query.AvailableQueries[metricRequest.GetName()]
	goodQuery, totalQuery := queryStruct.GetGoodQuery(), queryStruct.GetTotalQuery()

	if goodQuery.GetMetricFilter() == totalQuery.GetMetricFilter() {
		id, err := doDownstreamQuery(p, ctx, metricRequest, goodQuery)
		if err != nil {
			return "", "", err
		}
		return id, id, nil
	} else {
		idGood, err := doDownstreamQuery(p, ctx, metricRequest, goodQuery)
		if err != nil {
			return "", "", err
		}
		idTotal, err := doDownstreamQuery(p, ctx, metricRequest, totalQuery)
		if err != nil {
			return "", "", err
		}
		return idGood, idTotal, nil
	}
}

// Note: Assumption is that JobID is valid
// @returns goodQuery, totalQuery
func fetchPreconfQueries(slo *api.ServiceLevelObjective, service *api.Service, ctx context.Context, lg hclog.Logger) (*query.SLOQueryResult, error) {
	if slo.GetDatasource() == shared.MonitoringDatasource {
		if _, ok := query.AvailableQueries[service.GetMetricName()]; !ok {
			return nil, shared.ErrInvalidMetric
		}
		ratioQuery, err := query.AvailableQueries[service.GetMetricName()].Construct(service)
		if err != nil {
			return nil, err
		}
		return ratioQuery, nil
	} else if slo.GetDatasource() == shared.LoggingDatasource {
		return nil, shared.ErrNotImplemented
	} else {
		return nil, shared.ErrInvalidDatasource
	}
}
