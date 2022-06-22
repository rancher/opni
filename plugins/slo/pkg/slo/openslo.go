package slo

import (
	"context"
	"fmt"
	"strconv"

	"github.com/OpenSLO/oslo/pkg/manifest"
	oslov1 "github.com/OpenSLO/oslo/pkg/manifest/v1"
	"github.com/hashicorp/go-hclog"
	api "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
)

const (
	// Metric Enum
	MetricLatency      = "http-latency"
	MetricAvailability = "http-availability"

	// Datasource Enum
	LoggingDatasource    = "logging"
	MonitoringDatasource = "monitoring"

	// Alert Enum
	AlertingBurnRate = "burnrate"
	AlertingBudget   = "budget"

	// Notification Enum
	NotifSlack = "slack"
	NotifMail  = "email"
	NotifPager = "pager"

	osloVersion = "openslo/v1"
)

/// Returns a list of all the components passed in the protobuf we need to translate to specs
/// @errors: slo must have an id
func ParseToOpenSLO(slo *api.ServiceLevelObjective, ctx context.Context, lg hclog.Logger) ([]manifest.OpenSLOKind, error) {
	res := make([]manifest.OpenSLOKind, 0)
	// Parse to inline SLO/SLIs
	newSLOI := oslov1.SLOSpec{
		Description:     slo.GetDescription(),
		Service:         slo.GetServiceId(),
		BudgetingMethod: slo.GetMonitorWindow(),
	}
	// actual SLO/SLI query
	indicator, err := ParseToIndicator(slo, ctx, lg)
	if err != nil {
		return res, err
	}
	newSLOI.Indicator = indicator

	// targets
	newSLOI.Objectives = ParseToObjectives(slo, ctx, lg)
	wrapSLOI := oslov1.SLO{
		ObjectHeader: manifest.ObjectHeader{APIVersion: osloVersion}, //FIXME: subject to change in near future API
		Spec:         newSLOI,
	}

	// Parse inline Alert Policies and Alert Notifications
	notifs, policies, err := ParseToAlerts(slo, ctx, lg)
	if err != nil {
		return nil, err
	}
	for _, n := range notifs {
		res = append(res, n)
	}

	for _, p := range policies {
		res = append(res, p)
	}

	res = append(res, wrapSLOI)

	return res, nil
}

/// @note : for now only one indicator per SLO is supported
/// Indicator is OpenSLO's inline indicator
func ParseToIndicator(slo *api.ServiceLevelObjective, ctx context.Context, lg hclog.Logger) (*oslov1.SLIInline, error) {
	metadata := oslov1.Metadata{
		Name: fmt.Sprintf("sli-%s-%s", slo.GetMetricType(), slo.GetName()),
	}
	metric_type := ""
	if slo.GetDatasource() == MonitoringDatasource {

		metric_type = "prometheus" // OpenSLO standard
	} else {
		metric_type = "Opni" // Not OpenSLO standard, but custom
	}
	metric_query_bad, metric_query_total, err := fetchPreconfQueries(slo, ctx, lg)

	if err != nil {
		return nil, err
	}

	bad_metric := oslov1.MetricSource{
		Type:             metric_type,
		MetricSourceSpec: map[string]string{"query": metric_query_bad}, // this is flimsy
	}
	total_metric := oslov1.MetricSource{
		Type:             metric_type,
		MetricSourceSpec: map[string]string{"query": metric_query_total}, // this is flimsy
	}
	spec := oslov1.SLISpec{
		RatioMetric: &oslov1.RatioMetric{
			Counter: true, //MAYBE : should not always be true
			Bad:     &oslov1.MetricSourceHolder{MetricSource: bad_metric},
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
			Target:          float64(target.GetValueX100() / 100),
			TimeSliceWindow: slo.GetMonitorWindow(),
		}
		objectives = append(objectives, newObjective)
	}
	return objectives
}

func ParseToAlerts(slo *api.ServiceLevelObjective, ctx context.Context, lg hclog.Logger) ([]oslov1.AlertPolicy, []oslov1.AlertNotificationTarget, error) {
	policies := make([]oslov1.AlertPolicy, 0)
	notifs := make([]oslov1.AlertNotificationTarget, 0)

	for _, alert := range slo.Alerts {
		// Create noticiation targets, and then store their refs in policy specs
		target_spec := oslov1.AlertNotificationTargetSpec{
			Target:      alert.GetNotificationTarget(),
			Description: alert.GetDescription(),
		}

		target := oslov1.AlertNotificationTarget{
			ObjectHeader: manifest.ObjectHeader{APIVersion: osloVersion}, //FIXME: subject to change in near future API
			Spec:         target_spec,
		}

		// Create policy with inline condition
		policy_spec := oslov1.AlertPolicySpec{
			Description:         alert.GetDescription(),
			AlertWhenNoData:     alert.GetOnNoData(),
			AlertWhenBreaching:  alert.GetOnBreach(),
			AlertWhenResolved:   alert.GetOnResolved(),
			Conditions:          []oslov1.AlertPolicyCondition{},
			NotificationTargets: []oslov1.AlertPolicyNotificationTarget{},
		}

		wrapPolicy := oslov1.AlertPolicy{
			ObjectHeader: manifest.ObjectHeader{APIVersion: osloVersion}, //FIXME: subject to change in near future API
			Spec:         policy_spec,
		}
		policies = append(policies, wrapPolicy)
		notifs = append(notifs, target)
	}

	return policies, notifs, nil

}

func fetchPreconfQueries(slo *api.ServiceLevelObjective, ctx context.Context, lg hclog.Logger) (string, string, error) {
	// TODO : Refactor to template
	if slo.GetDatasource() == MonitoringDatasource {
		switch slo.GetDatasource() {
		case MetricAvailability:
			//Note: preconfigured status codes could be subject to change
			good_query := fmt.Sprintf("sum(rate(http_request_duration_seconds_count{job=\"%s\",code=~\"(2..|3..)\"}[{{.window}}]))", slo.GetServiceId())
			total_query := fmt.Sprintf("sum(rate(http_request_duration_seconds_count{job=\"%s\"}[{{.window}}]))", slo.GetServiceId())
			return good_query, total_query, nil
		case MetricLatency:
			// FIXME: untested with prometheus
			good_query := "sum(rate(http_request_duration_seconds_count{job=\"%s\",le:\"0.5\",verb!=\"WATCH\"}[{{.window}}]))"
			total_query := "sum(rate(apiserver_request_duration_seconds_count{verb!=\"WATCH\"}[{{.window}}]))"
			return good_query, total_query, nil
		default:
			return "", "", fmt.Errorf("unsupported metric type")
		}
	} else {
		return "", "", fmt.Errorf("preconfigured queries not implemented for datasource %s", slo.GetDatasource())
	}
}
