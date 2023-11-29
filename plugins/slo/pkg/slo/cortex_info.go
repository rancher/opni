package slo

import (
	"context"
	"fmt"
	"time"

	"emperror.dev/errors"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/metrics/compat"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
)

var instantMaskDisabled = true

func createGrafanaSLOMask(ctx context.Context, p *Plugin, clusterId string, ruleId string) error {
	lg := logger.PluginLoggerFromContext(p.ctx)

	lg.With("sloId", ruleId, "clusterId", clusterId).Debug("creating grafana mask")
	if !instantMaskDisabled {
		_, err := p.adminClient.Get().WriteMetrics(ctx, &cortexadmin.WriteRequest{
			ClusterID: clusterId,
			Timeseries: []*cortexadmin.TimeSeries{
				{
					Labels: []*cortexadmin.Label{
						{
							Name:  "__name__", //unique identifier for metrics
							Value: grafana_delete_mask,
						},
						{
							Name:  slo_uuid,
							Value: ruleId,
						},
					},
				},
			},
		})
		return err
	}
	return nil
}

func tryApplyThenDeleteCortexRules(
	ctx context.Context,
	p *Plugin,
	clusterId string,
	ruleId *string,
	toApply []rulefmt.RuleGroup,
) error {
	lg := logger.PluginLoggerFromContext(ctx)
	var errArr []error
	for _, rules := range toApply {
		err := applyCortexSLORules(
			ctx,
			p,
			clusterId,
			rules,
		)
		if err != nil {
			errArr = append(errArr, err)
		}
	}
	if len(errArr) > 0 {
		for _, rules := range toApply {
			err := deleteCortexSLORules(
				ctx,
				p,
				clusterId,
				rules.Name,
			)
			if err != nil {
				errArr = append(errArr, err)
			}
		}
	}
	if ruleId != nil {
		err := createGrafanaSLOMask(ctx, p, clusterId, *ruleId)
		if err != nil {
			lg.Error(fmt.Sprintf("creating grafana mask failed %s", err))
			errArr = append(errArr, err)
		}
	}

	return errors.Combine(errArr...)
}

// Apply Cortex Rules to Cortex separately :
// - recording rules
// - metadata rules
// - alert rules
func applyCortexSLORules(
	ctx context.Context,
	p *Plugin,
	clusterId string,
	ruleSpec rulefmt.RuleGroup,
) error {
	lg := logger.PluginLoggerFromContext(ctx)

	out, err := yaml.Marshal(ruleSpec)
	if err != nil {
		return err
	}

	_, err = p.adminClient.Get().LoadRules(ctx, &cortexadmin.LoadRuleRequest{
		Namespace:   "slo",
		YamlContent: out,
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
	ctx context.Context,
	p *Plugin,
	clusterId string,
	groupName string,
) error {
	_, err := p.adminClient.Get().DeleteRule(ctx, &cortexadmin.DeleteRuleRequest{
		ClusterId: clusterId,
		Namespace: "slo",
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
	ctx context.Context,
	client cortexadmin.CortexAdminClient,
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
	qres, err := compat.UnmarshalPrometheusResponse(rawBytes)
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
	ctx context.Context,
	client cortexadmin.CortexAdminClient,
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
	qres, err := compat.UnmarshalPrometheusResponse(rawBytes)
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
	ctx context.Context,
	client cortexadmin.CortexAdminClient,
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
	qres, err := compat.UnmarshalPrometheusResponse(rawBytes)
	if err != nil {
		return nil, err
	}
	dataMatrix, err := qres.GetMatrix()
	if err != nil {
		return nil, err
	}
	return dataMatrix, nil
}
