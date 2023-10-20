package slo

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"slices"

	"emperror.dev/errors"
	"github.com/google/uuid"
	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/apis/slo"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *SLOMonitoring) WithCurrentRequest(ctx context.Context, req proto.Message) SLOStore {
	s.req = req
	s.ctx = ctx
	return s
}

func (s SLOMonitoring) Create() (*corev1.Reference, error) {
	req := (s.req).(*sloapi.CreateSLORequest)
	slo := CreateSLORequestToStruct(req)
	rrecording, rmetadata, ralerting := slo.ConstructCortexRules(nil)
	toApply := []rulefmt.RuleGroup{rrecording, rmetadata, ralerting}
	ruleId := slo.GetId()
	err := tryApplyThenDeleteCortexRules(s.ctx, s.p, s.p.logger, req.GetSlo().GetClusterId(), &ruleId, toApply)
	if err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: slo.GetId()}, nil
}

func (s SLOMonitoring) Update(existing *sloapi.SLOData) (*sloapi.SLOData, error) {
	incomingSLO := (s.req).(*sloapi.SLOData) // Create is the same as Update if within the same cluster
	newSlo := SLODataToStruct(incomingSLO)
	rrecording, rmetadata, ralerting := newSlo.ConstructCortexRules(nil)
	toApply := []rulefmt.RuleGroup{rrecording, rmetadata, ralerting}
	err := tryApplyThenDeleteCortexRules(s.ctx, s.p, s.p.logger, incomingSLO.GetSLO().GetClusterId(), nil, toApply)

	// successfully applied rules to another cluster
	if err == nil && existing.SLO.ClusterId != incomingSLO.SLO.ClusterId {
		_, err := s.p.DeleteSLO(s.ctx, &corev1.Reference{Id: existing.Id})
		if err != nil {
			s.lg.With("sloId", existing.Id).Error(fmt.Sprintf(
				"Unable to delete SLO when updating between clusters :  %v",
				err))
		}
	}
	return incomingSLO, err
}

func (s SLOMonitoring) Delete(existing *sloapi.SLOData) error {
	id, clusterId := existing.Id, existing.SLO.ClusterId
	//err := deleteCortexSLORules(s.p, id, clusterId, s.ctx, s.lg)
	errArr := []error{}
	slo := SLODataToStruct(existing)
	rrecording, rmetadata, ralerting := slo.ConstructCortexRules(nil)
	toApply := []rulefmt.RuleGroup{rrecording, rmetadata, ralerting}
	for _, ruleName := range toApply {
		for _, rule := range ruleName.Rules {
			if rule.Alert.Value != "" {
				err := deleteCortexSLORules(
					s.ctx,
					s.p,
					s.p.logger,
					clusterId,
					rule.Alert.Value,
				)
				if err != nil {
					errArr = append(errArr, err)
				}
			}
			if rule.Record.Value != "" {
				err := deleteCortexSLORules(
					s.ctx,
					s.p,
					s.p.logger,
					clusterId,
					rule.Record.Value,
				)
				if err != nil {
					errArr = append(errArr, err)
				}
			}
		}
	}
	err := createGrafanaSLOMask(s.ctx, s.p, clusterId, id)
	if err != nil {
		s.p.logger.Error(fmt.Sprintf("creating grafana mask failed %s", err))
		errArr = append(errArr, err)
	}
	return errors.Combine(errArr...)
}

func (s SLOMonitoring) Clone(clone *sloapi.SLOData) (*corev1.Reference, *sloapi.SLOData, error) {
	clonedData := util.ProtoClone(clone)
	sloData := clonedData.GetSLO()
	slo := SLODataToStruct(clonedData)
	slo.SetId(uuid.New().String())
	slo.SetName(sloData.GetName() + "-clone")
	rrecording, rmetadata, ralerting := slo.ConstructCortexRules(nil)
	toApply := []rulefmt.RuleGroup{rrecording, rmetadata, ralerting}
	ruleId := slo.GetId()
	err := tryApplyThenDeleteCortexRules(s.ctx, s.p, s.p.logger, sloData.GetClusterId(), &ruleId, toApply)
	clonedData.SLO.Name = sloData.Name + "-clone"
	clonedData.Id = slo.GetId()
	return &corev1.Reference{Id: slo.GetId()}, clonedData, err
}

func (s SLOMonitoring) MultiClusterClone(
	base *sloapi.SLOData,
	inputClusters []*corev1.Reference,
	svcBackend ServiceBackend,
) ([]*corev1.Reference, []*sloapi.SLOData, []error) {
	clonedData := util.ProtoClone(base)
	sloData := clonedData.GetSLO()
	slo := SLODataToStruct(clonedData)

	clusters, err := s.p.mgmtClient.Get().ListClusters(s.ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, nil, []error{err}
	}
	var clusterIds []string

	for _, cluster := range clusters.Items {
		clusterIds = append(clusterIds, cluster.Id)
	}
	clusterDefinitions := make([]*sloapi.SLOData, len(inputClusters))
	clusterIdsCreate := make([]*corev1.Reference, len(inputClusters))
	errArr := make([]error, len(inputClusters))
	var wg sync.WaitGroup
	for idx, clusterId := range inputClusters {
		wg.Add(1)
		slo.SetId(uuid.New().String())
		slo.SetName(fmt.Sprintf("%s-clone-%d", sloData.GetName(), idx))
		rrecording, rmetadata, ralerting := slo.ConstructCortexRules(nil)
		toApply := []rulefmt.RuleGroup{rrecording, rmetadata, ralerting}
		// capture in closure
		idx := idx
		clusterId := clusterId
		ruleId := slo.GetId()
		if !slices.Contains(clusterIds, clusterId.Id) {
			errArr[idx] = fmt.Errorf("cluster %s not found", clusterId.Id)
			continue
		}
		svcBackend.WithCurrentRequest(s.ctx, &sloapi.ListServicesRequest{
			Datasource: "monitoring",
			ClusterId:  clusterId.Id,
		})
		services, err := svcBackend.ListServices()
		if err != nil {
			errArr[idx] = err
			continue
		}
		if services.ContainsId(sloData.GetServiceId()) {
			errArr[idx] = fmt.Errorf("service %s not found on cluster %s", sloData.GetServiceId(), clusterId.Id)
			continue
		}
		svcBackend.WithCurrentRequest(s.ctx, &sloapi.ListMetricsRequest{
			Datasource: "monitoring",
			ClusterId:  clusterId.Id,
			ServiceId:  sloData.GetServiceId(),
		})
		metrics, err := svcBackend.ListMetrics()
		if err != nil {
			errArr[idx] = err
			continue
		}
		if !metrics.ContainsId(sloData.GetGoodMetricName()) {
			errArr[idx] = fmt.Errorf(
				"good metric %s not found on cluster %s",
				sloData.GetGoodMetricName(),
				clusterId.Id,
			)
			continue
		}
		if !metrics.ContainsId(sloData.GetTotalMetricName()) {
			errArr[idx] = fmt.Errorf(
				"total metric %s not found on cluster %s",
				sloData.GetTotalMetricName(),
				clusterId.Id,
			)
			continue
		}
		errArr[idx] = tryApplyThenDeleteCortexRules(s.ctx, s.p, s.p.logger, clusterId.Id, &ruleId, toApply)
		clonedData.SLO.Name = sloData.Name + "-clone-" + strconv.Itoa(idx)
		clonedData.Id = slo.GetId()
		clusterDefinitions[idx] = clonedData
		clusterIdsCreate[idx] = &corev1.Reference{Id: slo.GetId()}
	}
	return clusterIdsCreate, clusterDefinitions, errArr
}

// Status Only return errors here that should be considered severe InternalServerErrors
// - Check if enough time has passed to evaluate the rules
// - First Checks if it has NoData
// - If it has Data, check if it is within budget
// - If is within budget, check if any alerts are firing
func (s SLOMonitoring) Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error) {
	now := time.Now()

	if now.Sub(existing.CreatedAt.AsTime()) <= sloapi.MinEvaluateInterval*2 {
		s.lg.With("sloId", existing.Id).Debug("SLO status is not ready to be evaluated : ",
			(&sloapi.SLOStatus{State: sloapi.SLOStatusState_Creating}).String())

		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_Creating}, nil
	}
	state := sloapi.SLOStatusState_Ok
	slo := SLODataToStruct(existing)
	// ======================= sli =======================
	sliErrorName := slo.ConstructRecordingRuleGroup(nil).Rules[0].Record
	sliDataVector, err := QuerySLOComponentByRecordName(
		s.ctx,
		s.p.adminClient.Get(),
		sliErrorName.Value,
		existing.GetSLO().GetClusterId(),
	)
	if err != nil {
		return nil, err
	}
	if sliDataVector == nil || sliDataVector.Len() == 0 {
		return &sloapi.SLOStatus{State: sloapi.SLOStatusState_NoData}, nil
	}
	s.lg.With("sloId", slo.GetId()).Debug(fmt.Sprintf("sli status response vector : %s", sliDataVector.String()))
	// ======================= error budget =======================
	// race condition can cause initial evaluation to fail with empty vector, resulting in no data state
	// this is why we return creating state with two intervals
	metadataBudgetRaw := slo.RawBudgetRemainingQuery() // this is not actually raw "raw", contains recording rule refs
	metadataVector, err := QuerySLOComponentByRawQuery(s.ctx, s.p.adminClient.Get(), metadataBudgetRaw, existing.GetSLO().GetClusterId())
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
	s.lg.With("sloId", slo.GetId()).Debug(fmt.Sprintf("sli status %s", metadataVector.String()))
	//
	//// ======================= alert =======================

	alertBudgetRules := slo.ConstructAlertingRuleGroup(nil)
	short, long := alertBudgetRules.Rules[0].Expr.Value, alertBudgetRules.Rules[1].Expr.Value
	alertDataVector1, err := QuerySLOComponentByRawQuery(s.ctx, s.p.adminClient.Get(), short, existing.GetSLO().GetClusterId())
	if err != nil {
		return nil, err
	}
	alertDataVector2, err := QuerySLOComponentByRawQuery(s.ctx, s.p.adminClient.Get(), long, existing.GetSLO().GetClusterId())
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
			Objective: normalizeObjective(req.GetSlo().GetTarget().GetValue()),
			Items:     []*sloapi.DataPoint{},
			Windows:   []*sloapi.AlertFiringWindows{},
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
	if step < time.Second {
		step = time.Second
	}

	ruleGroup := slo.ConstructRecordingRuleGroup(lo.ToPtr(sloapi.MinEvaluateInterval))
	sliPeriodErrorRate := ruleGroup.Rules[len(ruleGroup.Rules)-1].Expr.Value
	sli := "1 - (max(" + sliPeriodErrorRate + ") OR on() vector(NaN))" // handles the empty case and still differentiates between 0 and empty
	_, err = parser.ParseExpr(sli)
	if err != nil {
		panic(err)
	}
	sliDataMatrix, err := QuerySLOComponentByRawQueryRange(s.ctx, s.p.adminClient.Get(),
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
				Sli:       float64(yieldedValue.Value) * 100,
			})
		}
	}

	alertCriticalRawQuery, alertSevereRawQuery := slo.ConstructRawAlertQueries()
	// ideally should be every 5 minutes for fine grained detail
	// but for performance reasons, we will only query every 20 minutes
	alertTimeStep := time.Minute * 20

	alertWindowSevereMatrix, err := QuerySLOComponentByRawQueryRange(s.ctx, s.p.adminClient.Get(),
		alertSevereRawQuery.Value, req.GetSlo().GetClusterId(),
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

	alertWindowCriticalMatrix, err := QuerySLOComponentByRawQueryRange(s.ctx, s.p.adminClient.Get(),
		alertCriticalRawQuery.Value, req.GetSlo().GetClusterId(),
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

func (m *MonitoringServiceBackend) WithCurrentRequest(ctx context.Context, req proto.Message) ServiceBackend {
	m.req = req
	m.ctx = ctx
	return m
}

func (m MonitoringServiceBackend) ListServices() (*sloapi.ServiceList, error) {
	req := m.req.(*sloapi.ListServicesRequest)
	res := &sloapi.ServiceList{}
	discoveryQuery := `group by(job) ({__name__!=""})`
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
	res := &sloapi.EventList{
		Items: []*sloapi.Event{},
	}
	req := (m.req).(*sloapi.ListEventsRequest) // Create is the same as Update if within the same cluster
	resp, err := m.p.adminClient.Get().GetMetricLabelSets(m.ctx, &cortexadmin.LabelRequest{
		Tenant:     req.GetClusterId(),
		JobId:      req.GetServiceId(),
		MetricName: req.GetMetricId(),
	})
	if err != nil {
		return nil, err
	}
	for _, item := range resp.GetItems() {
		res.Items = append(res.Items, &sloapi.Event{
			Key:  item.GetName(),
			Vals: item.GetItems(),
		})
	}
	return res, nil
}

func (m MonitoringServiceBackend) ListMetrics() (*sloapi.MetricGroupList, error) {
	req := (m.req).(*sloapi.ListMetricsRequest) // Create is the same as Update if within the same cluster
	resp, err := m.p.adminClient.Get().GetSeriesMetrics(m.ctx, &cortexadmin.SeriesRequest{
		Tenant: req.GetClusterId(),
		JobId:  req.GetServiceId(),
	})
	if err != nil {
		return nil, err
	}
	return ApplyFiltersToCortexEvents(resp)
}
