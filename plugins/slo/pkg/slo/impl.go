package slo

import (
	"context"
	"fmt"
	promql "github.com/cortexproject/cortex/pkg/configs/legacy_promql"
	"github.com/google/uuid"
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

func (s SLOMonitoring) Create() (*corev1.Reference, error) {
	req := (s.req).(*sloapi.CreateSLORequest)
	if req.Slo.GetGoodMetricName() == req.Slo.GetTotalMetricName() {
		req.Slo.GoodEvents, req.Slo.TotalEvents = ToMatchingSubsetIdenticalMetric(req.Slo.GoodEvents, req.Slo.TotalEvents)
	}
	slo := CreateSLORequestToStruct(req)
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
	return &corev1.Reference{Id: slo.GetId()}, anyError
}

func (s SLOMonitoring) Update(existing *sloapi.SLOData) (*sloapi.SLOData, error) {
	req := (s.req).(*sloapi.SLOData) // Create is the same as Update if within the same cluster
	if req.SLO.GetGoodMetricName() == req.SLO.GetTotalMetricName() {
		req.SLO.GoodEvents, req.SLO.TotalEvents = ToMatchingSubsetIdenticalMetric(req.SLO.GoodEvents, req.SLO.TotalEvents)
	}
	newSlo := SLODataToStruct(req)
	rrecording, rmetadata, ralerting := newSlo.ConstructCortexRules(nil)
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
	// clean up old rules
	if anyError == nil && existing.SLO.ClusterId != req.SLO.ClusterId {
		_, err := s.p.DeleteSLO(s.ctx, &corev1.Reference{Id: req.Id})
		if err != nil {
			s.lg.With("sloId", req.Id).Error(fmt.Sprintf(
				"Unable to delete SLO when updating between clusters :  %v",
				err))
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
	sloData := clonedData.GetSLO()
	slo := SLODataToStruct(clonedData)
	slo.SetId(uuid.New().String())
	slo.SetName(sloData.GetName() + "-clone")
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
	evaluationInterval := time.Minute
	if now.Sub(existing.CreatedAt.AsTime()) <= evaluationInterval*2 {
		s.lg.With("sloId", existing.Id).Debug("SLO status is not ready to be evaluated : ",
			(&sloapi.SLOStatus{State: sloapi.SLOStatusState_Creating}).String())

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
	s.lg.With("sloId", slo.GetId()).Debug("sli status response vector : ", sliDataVector.String())
	// ======================= error budget =======================
	// race condition can cause initial evaluation to fail with empty vector, resulting in no data state
	// this is why we return creating state with two intervals
	metadataBudgetRaw := slo.RawBudgetRemainingQuery() // this is not actually raw "raw", contains recording rule refs
	metadataVector, err := QuerySLOComponentByRawQuery(s.p.adminClient.Get(), s.ctx, metadataBudgetRaw, existing.GetSLO().GetClusterId())
	if err != nil {
		return nil, err
	}
	if metadataVector == nil || metadataVector.Len() == 0 {
		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_PartialDataOk}, nil
	}
	metadataBudget := (*metadataVector)[0].Value
	if metadataBudget <= 0 {
		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_Breaching}, nil
	}
	s.lg.With("sloId", slo.GetId()).Debug("sli status ", metadataVector.String())
	//
	//// ======================= alert =======================

	alertBudgetRules := slo.ConstructAlertingRuleGroup(nil)
	short, long := alertBudgetRules.Rules[0].Expr, alertBudgetRules.Rules[1].Expr
	alertDataVector1, err := QuerySLOComponentByRawQuery(s.p.adminClient.Get(), s.ctx, short, existing.GetSLO().GetClusterId())
	if err != nil {
		return nil, err
	}
	alertDataVector2, err := QuerySLOComponentByRawQuery(s.p.adminClient.Get(), s.ctx, long, existing.GetSLO().GetClusterId())
	if err != nil {
		return nil, err
	}
	if alertDataVector1 == nil || alertDataVector1.Len() == 0 || alertDataVector2 == nil || alertDataVector2.Len() == 0 {
		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_PartialDataOk}, nil
	}
	if (*alertDataVector1)[len(*alertDataVector1)-1].Value > 0 || (*alertDataVector2)[len(*alertDataVector2)-1].Value > 0 {
		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_Warning}, nil
	}
	s.lg.With("sloId", slo.GetId()).Debug("alert status response vector ", alertDataVector1.String(), alertDataVector2.String())
	return &sloapi.SLOStatus{
		State: state,
	}, nil
}

func (s SLOMonitoring) Preview(slo *SLO) (*sloapi.SLOPreviewResponse, error) {
	req := s.req.(*sloapi.CreateSLORequest)
	preview := &sloapi.SLOPreviewResponse{
		PlotVector: &sloapi.PlotVector{
			Items:   []*sloapi.DataPoint{},
			Windows: []*sloapi.AlertFiringWindows{},
		},
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
	sli := "1 - (max(" + sliPeriodErrorRate + ") OR on() vector(1))"
	_, err = promql.ParseExpr(sli)
	if err != nil {
		panic(err)
	}
	sliDataMatrix, err := QuerySLOComponentByRawQueryRange(s.p.adminClient.Get(), s.ctx,
		sli, req.GetSlo().GetClusterId(),
		startTs, endTs, step,
	)
	if err != nil {
		return nil, err
	}
	for _, sample := range *sliDataMatrix {
		for _, yieldedValue := range sample.Values {
			ts := time.Unix(yieldedValue.Timestamp.Unix(), 0)
			preview.PlotVector.Items = append(preview.PlotVector.Items, &sloapi.DataPoint{
				Timestamp: timestamppb.New(ts),
				Sli:       float64(yieldedValue.Value),
				Objective: req.Slo.GetTarget().GetValue() / 100,
			})
		}
	}

	alertSevereRawQuery, alertCriticalRawQuery := slo.ConstructRawAlertQueries()
	// ideally should be every 5 minutes for fine grained detail
	// but for performance reasons, we will only query every 20 minutes
	alertTimeStep := time.Minute * 20

	alertWindowSevereMatrix, err := QuerySLOComponentByRawQueryRange(s.p.adminClient.Get(), s.ctx,
		alertSevereRawQuery, req.GetSlo().GetClusterId(),
		startTs, endTs, alertTimeStep,
	)
	if err != nil {
		return nil, err
	}
	severeWindows, err := DetectActiveWindows("severe", alertWindowSevereMatrix)
	if err != nil {
		return nil, err
	}
	preview.PlotVector.Windows = append(preview.PlotVector.Windows, severeWindows...)

	alertWindowCriticalMatrix, err := QuerySLOComponentByRawQueryRange(s.p.adminClient.Get(),
		s.ctx, alertCriticalRawQuery, req.GetSlo().GetClusterId(),
		startTs, endTs, step,
	)
	if err != nil {
		return nil, err
	}
	criticalWindows, err := DetectActiveWindows("critical", alertWindowCriticalMatrix)
	if err != nil {
		return nil, err
	}
	preview.PlotVector.Windows = append(preview.PlotVector.Windows, criticalWindows...)
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
