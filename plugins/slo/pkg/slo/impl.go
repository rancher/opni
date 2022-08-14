package slo

import (
	"context"
	"fmt"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/slo/shared"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
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
//
// - First Checks if it has NoData
// - If it has Data, check if it is within budget
// - If is within budget, check if any alerts are firing
func (s SLOMonitoring) Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error) {
	return nil, shared.ErrNotImplemented
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
			Tenants: []string{req.GetClusterdId()},
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
			ClusterId: req.GetClusterdId(),
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
