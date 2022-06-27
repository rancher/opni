package slo

import (
	"context"
	"fmt"
	"strconv"

	"github.com/alexandreLamarre/oslo/pkg/manifest"
	oslov1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/hashicorp/go-hclog"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

/// Returns a list of all the components passed in the protobuf we need to translate to specs
/// @errors: slo must have an id
func ParseToOpenSLO(slo *api.ServiceLevelObjective, ctx context.Context, lg hclog.Logger) ([]oslov1.SLO, error) {
	res := make([]oslov1.SLO, 0)

	for idx, service := range slo.GetServices() {
		// Parse to inline SLO/SLIs
		newSLOI := oslov1.SLOSpec{
			Description:     slo.GetDescription(),
			Service:         service.GetJobId(),
			BudgetingMethod: slo.GetMonitorWindow(),
		}
		// actual SLO/SLI query
		indicator, err := ParseToIndicator(slo, service.GetJobId(), service.GetMetricName(), service.GetMetricId(), ctx, lg)
		if err != nil {
			return res, err
		}
		newSLOI.Indicator = indicator

		// targets
		newSLOI.Objectives = ParseToObjectives(slo, ctx, lg)

		// Parse inline Alert Policies and Alert Notifications
		policies, err := ParseToAlerts(slo, ctx, lg)
		if err != nil {
			return nil, err
		}

		newSLOI.AlertPolicies = policies

		wrapSLOI := oslov1.SLO{
			ObjectHeader: oslov1.ObjectHeader{
				ObjectHeader: manifest.ObjectHeader{APIVersion: osloVersion},
				Kind:         "SLO",
			},
			Spec: newSLOI,
		}

		//Label SLO
		wrapSLOI.Metadata.Name = fmt.Sprintf("slo-%s-%d-%s-%s", slo.GetName(), idx, service.GetClusterId(), service.GetJobId())
		res = append(res, wrapSLOI)
	}

	return res, nil
}

/// @note : for now only one indicator per SLO is supported
/// Indicator is OpenSLO's inline indicator
func ParseToIndicator(slo *api.ServiceLevelObjective, jobId string, metricName string, metricId string, ctx context.Context, lg hclog.Logger) (*oslov1.SLIInline, error) {
	metadata := oslov1.Metadata{
		Name: fmt.Sprintf("sli-%s", slo.GetName()),
	}
	metric_type := ""
	if slo.GetDatasource() == MonitoringDatasource {

		metric_type = "prometheus" // OpenSLO standard
	} else {
		metric_type = "opni" // Not OpenSLO standard, but custom
	}
	ratioQuery, err := fetchPreconfQueries(slo, jobId, metricName, metricId, ctx, lg)

	if err != nil {
		return nil, err
	}

	good_metric := oslov1.MetricSource{
		Type:             metric_type,
		MetricSourceSpec: map[string]string{"query": ratioQuery.GoodQuery, "queryType": "promql"},
	}
	total_metric := oslov1.MetricSource{
		Type:             metric_type,
		MetricSourceSpec: map[string]string{"query": ratioQuery.TotalQuery, "queryType": "promql"},
	}
	spec := oslov1.SLISpec{
		RatioMetric: &oslov1.RatioMetric{
			Counter: true, //MAYBE : should not always be true in the future
			Good:    &oslov1.MetricSourceHolder{MetricSource: good_metric},
			Total:   oslov1.MetricSourceHolder{MetricSource: total_metric},
		},
	}
	SLI := oslov1.SLIInline{
		Metadata: metadata,
		Spec:     spec,
	}
	return &SLI, err
}

func ParseToObjectives(slo *api.ServiceLevelObjective, ctx context.Context, lg hclog.Logger) []oslov1.Objective {
	objectives := make([]oslov1.Objective, 0)
	for i, target := range slo.GetTargets() {
		newObjective := oslov1.Objective{
			DisplayName:     slo.GetName() + "-target" + strconv.Itoa(i),
			Target:          float64(target.GetValueX100()) / 100,
			TimeSliceWindow: slo.GetBudgetingInterval(),
		}
		objectives = append(objectives, newObjective)
	}
	return objectives
}

func ParseToAlerts(slo *api.ServiceLevelObjective, ctx context.Context, lg hclog.Logger) ([]oslov1.AlertPolicy, error) {
	policies := make([]oslov1.AlertPolicy, 0)

	for _, alert := range slo.Alerts {
		// Create noticiation targets, and then store their refs in policy specs
		target_spec := oslov1.AlertNotificationTargetSpec{
			Target:      alert.GetNotificationTarget(),
			Description: alert.GetDescription(),
		}

		target := oslov1.AlertNotificationTarget{
			ObjectHeader: oslov1.ObjectHeader{
				ObjectHeader: manifest.ObjectHeader{APIVersion: osloVersion},
				Kind:         "AlertNotificationTarget",
			},
			Spec: target_spec,
		}

		//TODO(alex) : handle alert conditions

		policy_spec := oslov1.AlertPolicySpec{
			Description:         alert.GetDescription(),
			AlertWhenNoData:     alert.GetOnNoData(),
			AlertWhenBreaching:  alert.GetOnBreach(),
			AlertWhenResolved:   alert.GetOnResolved(),
			Conditions:          []oslov1.AlertPolicyCondition{},
			NotificationTargets: []oslov1.AlertNotificationTarget{},
		}
		policy_spec.NotificationTargets = append(policy_spec.NotificationTargets, target)

		wrapPolicy := oslov1.AlertPolicy{
			ObjectHeader: oslov1.ObjectHeader{
				ObjectHeader: manifest.ObjectHeader{APIVersion: osloVersion},
				Kind:         "AlertPolicy",
			},
			Spec: policy_spec,
		}
		policies = append(policies, wrapPolicy)
	}

	return policies, nil

}
