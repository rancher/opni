package slo

import (
	"context"
	"fmt"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
	"time"
)

// Apply Cortex Rules to Cortex separately :
// - recording rules
// - metadata rules
// - alert rules
func applyCortexSLORules(
	p *Plugin,
	lg *zap.SugaredLogger,
	ctx context.Context,
	clusterId string,
	ruleSpec RuleGroupYAMLv2,
) error {
	out, err := yaml.Marshal(ruleSpec)
	if err != nil {
		return err
	}

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.PostRuleRequest{
		YamlContent: string(out),
		ClusterId:   clusterId,
	})
	if err != nil {
		lg.Error(fmt.Sprintf(
			"Failed to load rules for cluster %s, rule : %s,",
			clusterId, string(out)))
	}
	return err
}

// }
func deleteCortexSLORules(
	p *Plugin,
	lg *zap.SugaredLogger,
	ctx context.Context,
	clusterId string,
	groupName string,
) error {
	_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.RuleRequest{
		ClusterId: clusterId,
		GroupName: groupName,
	})
	// we can ignore 404s here since if we can't find them,
	// then it will be impossible to delete them anyway
	if status.Code(err) == codes.NotFound || status.Code(err) == codes.OK {
		return nil
	}
	return err
}

func QuerySLOComponentByRecordName(
	client cortexadmin.CortexAdminClient,
	ctx context.Context,
	recordName string,
	clusterId string,
) (*model.Vector, error) {
	resp, err := client.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{clusterId},
		Query:   recordName,
	})
	if err != nil {
		return nil, err
	}
	rawBytes := resp.Data
	qres, err := unmarshal.UnmarshalPrometheusResponse(rawBytes)
	if err != nil {
		return nil, err
	}
	dataVector, err := qres.GetVector()
	if err != nil {
		return nil, err
	}
	return dataVector, nil
}

func QuerySLOComponentByRawQuery(
	client cortexadmin.CortexAdminClient,
	ctx context.Context,
	rawQuery string,
	clusterId string,
) (*model.Vector, error) {
	resp, err := client.Query(ctx, &cortexadmin.QueryRequest{
		Tenants: []string{clusterId},
		Query:   rawQuery,
	})
	if err != nil {
		return nil, err
	}
	rawBytes := resp.Data
	qres, err := unmarshal.UnmarshalPrometheusResponse(rawBytes)
	if err != nil {
		return nil, err
	}
	dataVector, err := qres.GetVector()
	if err != nil {
		return nil, err
	}
	return dataVector, nil
}

func QuerySLOComponentByRawQueryRange(
	client cortexadmin.CortexAdminClient,
	ctx context.Context,
	rawQuery string,
	clusterId string,
	start time.Time,
	end time.Time,
	step time.Duration,
) (*model.Matrix, error) {
	resp, err := client.QueryRange(ctx, &cortexadmin.QueryRangeRequest{
		Tenants: []string{clusterId},
		Query:   rawQuery,
		Start:   timestamppb.New(start),
		End:     timestamppb.New(end),
		Step:    durationpb.New(step),
	})
	if err != nil {
		return nil, err
	}
	rawBytes := resp.Data
	qres, err := unmarshal.UnmarshalPrometheusResponse(rawBytes)
	if err != nil {
		return nil, err
	}
	dataMatrix, err := qres.GetMatrix()
	if err != nil {
		return nil, err
	}
	return dataMatrix, nil
}
