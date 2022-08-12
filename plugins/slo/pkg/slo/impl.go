package slo

import (
	"context"
	"fmt"
	v1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/slo/shared"
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
func (s SLOMonitoring) Create(osloSpecs []v1.SLO) (*corev1.ReferenceList, error) {
	//returnedSloId := &corev1.ReferenceList{}
	//req := (s.req).(*sloapi.CreateSLORequest)
	//openSpecServices, err := zipOpenSLOWithServices(osloSpecs, req.Services)
	//if err != nil {
	//	return nil, err
	//}
	//// possible for partial success, but don't want to exit on error
	//var anyError error
	//for _, zipped := range openSpecServices {
	//	// existingId="" if this is a new slo
	//	createdSlos, createError := applyMonitoringSLODownstream(*zipped.Spec, zipped.Service, "", s.p, req, s.ctx, s.lg)
	//
	//	if createError != nil {
	//		anyError = createError
	//	}
	//	for _, data := range createdSlos {
	//		// ONLY WHEN the SLO is applied, should we create the K,V
	//		if createError == nil {
	//			returnedSloId.Items = append(returnedSloId.Items, &corev1.Reference{Id: data.Id})
	//			if err := s.p.storage.Get().SLOs.Put(s.ctx, path.Join("/slos", data.Id), data); err != nil {
	//				return nil, err
	//			}
	//			if err != nil {
	//				anyError = err
	//			}
	//		}
	//	}
	//}
	return nil, shared.ErrNotImplemented
}

func (s SLOMonitoring) Update(osloSpecs []v1.SLO, existing *sloapi.SLOData) (*sloapi.SLOData, error) {
	//req := (s.req).(*sloapi.SLOData) // Create is the same as Update if within the same cluster
	//createReq := &sloapi.CreateSLORequest{
	//	SLO:      req.SLO,
	//	Services: []*sloapi.Service{req.Service},
	//}
	//
	//var anyError error
	//openSpecServices, err := zipOpenSLOWithServices(osloSpecs, []*sloapi.Service{req.Service})
	//if err != nil {
	//	return nil, err
	//}
	//// changing clusters means we need to clean up the rules on the old cluster
	//if existing.Service.ClusterId != req.Service.ClusterId {
	//	_, err := s.p.DeleteSLO(s.ctx, &corev1.Reference{Id: req.Id})
	//	if err != nil {
	//		s.lg.With("sloId", req.Id).Error(fmt.Sprintf(
	//			"Unable to delete SLO when updating between clusters :  %v",
	//			err))
	//	}
	//}
	//for _, zipped := range openSpecServices {
	//	// don't need creation metadata
	//	_, err := applyMonitoringSLODownstream(*zipped.Spec, zipped.Service,
	//		req.Id, s.p, createReq, s.ctx, s.lg)
	//
	//	if err != nil {
	//		anyError = err
	//	}
	//}
	//return req, anyError
	return nil, shared.ErrNotImplemented
}

func (s SLOMonitoring) Delete(existing *sloapi.SLOData) error {
	//id, clusterId := existing.Id, existing.Service.ClusterId
	//err := deleteCortexSLORules(s.p, id, clusterId, s.ctx, s.lg)
	return nil
}

func (s SLOMonitoring) Clone(clone *sloapi.SLOData) (string, error) {
	//var anyError error
	//createdSlos, err := s.p.CreateSLO(s.ctx, &sloapi.CreateSLORequest{
	//	SLO:      clone.SLO,
	//	Services: []*sloapi.Service{clone.Service},
	//})
	//if err != nil {
	//	return "", err
	//}
	//// should only create one slo
	//if len(createdSlos.Items) > 1 {
	//	anyError = status.Error(codes.Internal, "Created more than one SLO")
	//}
	//clone.Id = createdSlos.Items[0].Id
	//return clone.Id, anyError
	return "", shared.ErrNotImplemented
}

// Status Only return errors here that should be considered severe InternalServerErrors
//
// - First Checks if it has NoData
// - If it has Data, check if it is within budget
// - If is within budget, check if any alerts are firing
func (s SLOMonitoring) Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error) {
	//curState := sloapi.SLOStatusState_Ok
	//
	//// check if the recording rule has data
	//// rrecording, := existing.Id + RecordingRuleSuffix
	//rresp, err := s.p.adminClient.Get().Query(
	//	s.ctx,
	//	&cortexadmin.QueryRequest{
	//		Tenants: []string{existing.Service.ClusterId},
	//		Query:   fmt.Sprintf(`slo:sli_error:ratio_rate5m{%s="%s"}`, sloOpniIdLabel, existing.Id),
	//	},
	//)
	//if err != nil {
	//	s.lg.Error(fmt.Sprintf("Status : Got error for recording rule %v", err))
	//	return nil, err
	//}
	//q, err := unmarshal.UnmarshalPrometheusResponse(rresp.Data)
	//if err != nil {
	//	s.lg.Error(fmt.Sprintf("%v", err))
	//	return nil, err
	//}
	//switch q.V.Type() {
	//case model.ValVector:
	//	vv := q.V.(model.Vector)
	//	if len(vv) == 0 {
	//		curState = sloapi.SLOStatusState_NoData
	//	} else {
	//		curState = sloapi.SLOStatusState_Ok
	//	}
	//default: //FIXME: For now, return internal errors if we can't match result to a vector result
	//	s.lg.Error(fmt.Sprintf("Unexpected response type '%v' from Prometheus for recording rule", q.V.Type()))
	//	return &sloapi.SLOStatus{
	//		State: sloapi.SLOStatusState_InternalError,
	//	}, nil
	//
	//}
	//// Check if the metadata rules show we have breached the budget
	//// metadataRuleId := existing.Id + MetadataRuleSuffix
	//if curState == sloapi.SLOStatusState_Ok {
	//	_, err := s.p.adminClient.Get().Query(
	//		s.ctx,
	//		&cortexadmin.QueryRequest{
	//			Tenants: []string{existing.Service.ClusterId},
	//			Query:   "", // TODO : meaningful metadata queries here
	//		},
	//	)
	//	if err != nil {
	//		return &sloapi.SLOStatus{
	//			State: sloapi.SLOStatusState_InternalError,
	//		}, nil
	//	}
	//	// TODO : evaluate metadata rules
	//}
	//
	//if curState == sloapi.SLOStatusState_Ok {
	//	// Check if the conditions of any of the alerting rules are met
	//	// alertRuleId := existing.Id + AlertRuleSuffix
	//	_, err := s.p.adminClient.Get().Query(
	//		s.ctx,
	//		&cortexadmin.QueryRequest{
	//			Tenants: []string{existing.Service.ClusterId},
	//			Query:   "", // TODO : meaningful query to check alerting conditions here
	//		},
	//	)
	//	if err != nil {
	//		s.lg.Error(fmt.Sprintf("Status : Got error for recording rule %v", err))
	//	}
	//}
	//return &sloapi.SLOStatus{
	//	State: curState,
	//}, nil
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
