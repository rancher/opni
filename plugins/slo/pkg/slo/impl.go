package slo

import (
	"context"
	"fmt"
	promql "github.com/cortexproject/cortex/pkg/configs/legacy_promql"
	prommodel "github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func (s *SLOMonitoring) WithCurrentRequest(req proto.Message, ctx context.Context) SLOStore {
	s.req = req
	s.ctx = ctx
	return s
}

// Create OsloSpecs ----> sloth IR ---> Prometheus SLO --> Cortex Rule groups
func (s SLOMonitoring) Create(slo *SLO) error {
	req := (s.req).(*sloapi.CreateSLORequest)
	rrecording, rmetadata, ralerting := slo.ConstructCortexRules(nil)
	toApply := []RuleGroupYAMLv2{rrecording, rmetadata, ralerting}
	var anyError error
	for _, rules := range toApply {
		err := applyCortexSLORules(
			s.p,
			s.p.logger,
			s.ctx,
			req.GetSlo().GetClusterId(),
			rules,
		)
		if err != nil {
			anyError = err
		}
	}
	if anyError != nil {
		for _, rules := range toApply {
			err := deleteCortexSLORules(
				s.p,
				s.p.logger,
				s.ctx,
				req.GetSlo().GetClusterId(),
				rules.Name,
			)
			if err != nil {
				anyError = err
			}
		}
	}
	return anyError
}

func (s SLOMonitoring) Update(new *SLO, existing *sloapi.SLOData) (*sloapi.SLOData, error) {
	req := (s.req).(*sloapi.SLOData) // Create is the same as Update if within the same cluster
	if existing.SLO.ClusterId != req.SLO.ClusterId {
		_, err := s.p.DeleteSLO(s.ctx, &corev1.Reference{Id: req.Id})
		if err != nil {
			s.lg.With("sloId", req.Id).Error(fmt.Sprintf(
				"Unable to delete SLO when updating between clusters :  %v",
				err))
		}
	}
	rrecording, rmetadata, ralerting := new.ConstructCortexRules(nil)
	toApply := []RuleGroupYAMLv2{rrecording, rmetadata, ralerting}
	var anyError error
	for _, rules := range toApply {
		err := applyCortexSLORules(
			s.p,
			s.p.logger,
			s.ctx,
			req.GetSLO().GetClusterId(),
			rules,
		)
		if err != nil {
			anyError = err
		}
	}
	if anyError != nil {
		for _, rules := range toApply {
			err := deleteCortexSLORules(
				s.p,
				s.p.logger,
				s.ctx,
				req.GetSLO().GetClusterId(),
				rules.Name,
			)
			if err != nil {
				anyError = err
			}
		}
	}
	return req, anyError
}

func (s SLOMonitoring) Delete(existing *sloapi.SLOData) error {
	id, clusterId := existing.Id, existing.SLO.ClusterId
	//err := deleteCortexSLORules(s.p, id, clusterId, s.ctx, s.lg)
	var anyError error
	toApply := []string{id + RecordingRuleSuffix, id + MetadataRuleSuffix, id + AlertRuleSuffix}
	for _, ruleName := range toApply {
		err := deleteCortexSLORules(
			s.p,
			s.p.logger,
			s.ctx,
			clusterId,
			ruleName,
		)
		if err != nil {
			anyError = err
		}
	}
	return anyError
}

func (s SLOMonitoring) Clone(clone *sloapi.SLOData) (*corev1.Reference, *sloapi.SLOData, error) {
	clonedData := util.ProtoClone(clone)
	sloData := clone.GetSLO()
	sloLabels := map[string]string{}
	for _, label := range sloData.Labels {
		sloLabels[label.GetName()] = "true"
	}
	slo := NewSLO(
		sloData.GetName()+"-clone",
		sloData.GetSloPeriod(),
		sloData.GetTarget().GetValue(),
		Service(sloData.GetServiceId()),
		Metric(sloData.GetGoodMetricName()),
		Metric(sloData.GetTotalMetricName()),
		sloLabels,
		LabelPairs{}, //FIXME
		LabelPairs{}, //FIXME
	)
	rrecording, rmetadata, ralerting := slo.ConstructCortexRules(nil)
	toApply := []RuleGroupYAMLv2{rrecording, rmetadata, ralerting}
	var anyError error
	for _, rules := range toApply {
		err := applyCortexSLORules(
			s.p,
			s.p.logger,
			s.ctx,
			sloData.GetClusterId(),
			rules,
		)
		if err != nil {
			anyError = err
		}
	}
	if anyError != nil {
		for _, rules := range toApply {
			err := deleteCortexSLORules(
				s.p,
				s.p.logger,
				s.ctx,
				sloData.GetClusterId(),
				rules.Name,
			)
			if err != nil {
				anyError = err
			}
		}
	}
	clonedData.SLO.Name = sloData.Name + "-clone"
	clonedData.Id = slo.GetId()

	return &corev1.Reference{Id: slo.GetId()}, clonedData, anyError

}

// Status Only return errors here that should be considered severe InternalServerErrors
// - Check if enough time has passed to evaluate the rules
// - First Checks if it has NoData
// - If it has Data, check if it is within budget
// - If is within budget, check if any alerts are firing
func (s SLOMonitoring) Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error) {
	now := time.Now()
	if now.Sub(existing.CreatedAt.AsTime()) <= time.Minute {
		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_Creating}, nil
	}
	state := sloapi.SLOStatusState_Ok
	slo := SLODataToStruct(existing)
	// ======================= sli =======================
	sliErrorName := slo.ConstructRecordingRuleGroup(nil).Rules[0].Record
	sliDataVector, err := QuerySLOComponentByRecordName(
		s.p.adminClient.Get(),
		s.ctx,
		sliErrorName,
		existing.GetSLO().GetClusterId(),
	)
	if err != nil {
		return nil, err
	}
	if sliDataVector == nil || sliDataVector.Len() == 0 {
		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_NoData}, nil
	}
	// ======================= error budget =======================
	//FIXME: some sort of race condition here
	//metadataBudgetRaw := slo.RawBudgetRemainingQuery()
	//metadataVector, err := QuerySLOComponentByRawQuery(s.p.adminClient.Get(), s.ctx, metadataBudgetRaw, existing.GetSLO().GetClusterId())
	//if err != nil {
	//	return nil, err
	//}
	//if metadataVector == nil || metadataVector.Len() == 0 {
	//	return &sloapi.SLOStatus{State: sloapi.SLOStatusState_NoData}, nil
	//}
	//metadataBudget := (*metadataVector)[0].Value
	//if metadataBudget <= 0 {
	//	return &sloapi.SLOStatus{State: sloapi.SLOStatusState_Breaching}, nil
	//}
	//
	//// ======================= alert =======================
	//FIXME: alert vectors have no data
	//alertBudgetRules := slo.ConstructAlertingRuleGroup(nil)
	//short, long := alertBudgetRules.Rules[0].Alert, alertBudgetRules.Rules[1].Alert
	//alertDataVector1, err := QuerySLOComponentByRecordName(s.p.adminClient.Get(), s.ctx, short, existing.GetSLO().GetClusterId())
	//if err != nil {
	//	return nil, err
	//}
	//alertDataVector2, err := QuerySLOComponentByRecordName(s.p.adminClient.Get(), s.ctx, long, existing.GetSLO().GetClusterId())
	//if err != nil {
	//	return nil, err
	//}
	//if alertDataVector1 == nil || alertDataVector1.Len() == 0 || alertDataVector2 == nil || alertDataVector2.Len() == 0 {
	//	return &sloapi.SLOStatus{State: sloapi.SLOStatusState_NoData}, nil
	//}
	//if (*alertDataVector1)[len(*alertDataVector1)-1].Value > 0 || (*alertDataVector2)[len(*alertDataVector2)-1].Value > 0 {
	//	return &sloapi.SLOStatus{State: sloapi.SLOStatusState_Warning}, nil
	//}
	return &sloapi.SLOStatus{
		State: state,
	}, nil
}

func (s SLOMonitoring) Preview(slo *SLO) (*sloapi.SLOPreviewResponse, error) {
	req := s.req.(*sloapi.CreateSLORequest)
	preview := &sloapi.SLOPreviewResponse{
		SLI:          &sloapi.DataVector{},
		Objective:    &sloapi.DataVector{},
		Alerts:       &sloapi.DataVector{},
		SevereAlerts: &sloapi.DataVector{},
	}
	cur := time.Now()
	dur, err := prommodel.ParseDuration(slo.sloPeriod)
	if err != nil {
		panic(err)
	}
	startTs, endTs := cur.Add(time.Duration(-dur)), cur
	numSteps := 250
	step := time.Duration(endTs.Sub(startTs).Seconds()/float64(numSteps)) * time.Second

	ruleGroup := slo.ConstructRecordingRuleGroup(nil)
	sliPeriodErrorRate := ruleGroup.Rules[len(ruleGroup.Rules)-1].Expr
	sli := "1 - (max(" + sliPeriodErrorRate + ") OR on() vector(0))"
	_, err = promql.ParseExpr(sli)
	if err != nil {
		panic(err)
	}
	sliDataMatrix, err := QuerySLOComponentByRawQueryRange(
		s.p.adminClient.Get(),
		s.ctx,
		sli,
		req.GetSlo().GetClusterId(),
		startTs,
		endTs,
		step,
	)
	if err != nil {
		return nil, err
	}
	for _, sample := range *sliDataMatrix {
		for _, yieldedValue := range sample.Values {
			ts := time.Unix(int64(yieldedValue.Timestamp), 0)
			preview.SLI.Items = append(preview.SLI.Items, &sloapi.DataPoint{
				Timestamp: timestamppb.New(ts),
				Value:     float64(yieldedValue.Value),
			})
			preview.Objective.Items = append(preview.Objective.Items, &sloapi.DataPoint{
				Timestamp: timestamppb.New(ts),
				Value:     slo.GetObjective() / 100,
			})
		}
	}
	return preview, nil
}

func (m *MonitoringServiceBackend) WithCurrentRequest(req proto.Message, ctx context.Context) ServiceBackend {
	m.req = req
	m.ctx = ctx
	return m
}

func (m MonitoringServiceBackend) ListServices() (*sloapi.ServiceList, error) {
	req := m.req.(*sloapi.ListServicesRequest)
	res := &sloapi.ServiceList{}
	discoveryQuery := `group by(job)({__name__!=""})`
	resp, err := m.p.adminClient.Get().Query(
		m.ctx,
		&cortexadmin.QueryRequest{
			Tenants: []string{req.GetClusterId()},
			Query:   discoveryQuery,
		})
	if err != nil {
		return nil, err
	}
	result := gjson.Get(string(resp.Data), "data.result.#.metric.job")
	if !result.Exists() {
		return nil, fmt.Errorf("Could not convert prometheus service discovery to json ")
	}
	for _, v := range result.Array() {
		res.Items = append(res.Items, &sloapi.Service{
			ClusterId: req.GetClusterId(),
			ServiceId: v.String(),
		})
	}
	return res, nil
}

func (m MonitoringServiceBackend) ListEvents() (*sloapi.EventList, error) {
	req := (m.req).(*sloapi.ListEventsRequest) // Create is the same as Update if within the same cluster
	res := &sloapi.EventList{}
	resp, err := m.p.adminClient.Get().GetMetricLabelSets(m.ctx, &cortexadmin.LabelRequest{
		Tenant:     req.GetClusterId(),
		JobId:      req.GetServiceId(),
		MetricName: req.GetMetricId(),
	})
	if err != nil {
		return nil, err
	}
	for _, l := range resp.Items {
		res.Items = append(res.Items, &sloapi.Event{
			Key:  l.GetName(),
			Vals: l.GetItems(),
		})
	}
	return res, nil
}

func (m MonitoringServiceBackend) ListMetrics() (*sloapi.MetricList, error) {
	req := (m.req).(*sloapi.ListMetricsRequest) // Create is the same as Update if within the same cluster
	res := &sloapi.MetricList{}
	resp, err := m.p.adminClient.Get().GetSeriesMetrics(m.ctx, &cortexadmin.SeriesRequest{
		Tenant: req.GetClusterId(),
		JobId:  req.GetServiceId(),
	})
	if err != nil {
		return nil, err
	}
	for _, seriesInfo := range resp.Items {
		res.Items = append(res.Items, &sloapi.Metric{
			Id: seriesInfo.GetSeriesName(),
			Metadata: &sloapi.MetricMetadata{
				Description: seriesInfo.Metadata.GetDescription(),
				Unit:        seriesInfo.Metadata.GetUnit(),
				Type:        seriesInfo.Metadata.GetType(),
			},
		})
	}
	return res, nil
}
