package slo

import (
	"context"
	"fmt"
	"path"

	v1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	"github.com/prometheus/common/model"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s *SLOMonitoring) WithCurrentRequest(req proto.Message, ctx context.Context) SLOStore {
	s.req = req
	s.ctx = ctx
	return s
}

// Create OsloSpecs ----> sloth IR ---> Prometheus SLO --> Cortex Rule groups
func (s SLOMonitoring) Create(osloSpecs []v1.SLO) (*corev1.ReferenceList, error) {
	returnedSloId := &corev1.ReferenceList{}
	req := (s.req).(*sloapi.CreateSLORequest)
	openSpecServices, err := zipOpenSLOWithServices(osloSpecs, req.Services)
	if err != nil {
		return nil, err
	}
	// possible for partial success, but don't want to exit on error
	var anyError error
	for _, zipped := range openSpecServices {
		// existingId="" if this is a new slo
		createdSlos, createError := applyMonitoringSLODownstream(*zipped.Spec, zipped.Service, "", s.p, req, s.ctx, s.lg)

		if createError != nil {
			anyError = createError
		}
		for _, data := range createdSlos {
			// ONLY WHEN the SLO is applied, should we create the K,V
			if createError == nil {
				returnedSloId.Items = append(returnedSloId.Items, &corev1.Reference{Id: data.Id})
				if err := s.p.storage.Get().SLOs.Put(s.ctx, path.Join("/slos", data.Id), data); err != nil {
					return nil, err
				}
				if err != nil {
					anyError = err
				}
			}
		}
	}
	return returnedSloId, anyError
}

func (s SLOMonitoring) Update(osloSpecs []v1.SLO, existing *sloapi.SLOData) (*sloapi.SLOData, error) {
	req := (s.req).(*sloapi.SLOData) // Create is the same as Update if within the same cluster
	createReq := &sloapi.CreateSLORequest{
		SLO:      req.SLO,
		Services: []*sloapi.Service{req.Service},
	}

	var anyError error
	openSpecServices, err := zipOpenSLOWithServices(osloSpecs, []*sloapi.Service{req.Service})
	if err != nil {
		return nil, err
	}
	// changing clusters means we need to clean up the rules on the old cluster
	if existing.Service.ClusterId != req.Service.ClusterId {
		_, err := s.p.DeleteSLO(s.ctx, &corev1.Reference{Id: req.Id})
		if err != nil {
			s.lg.With("sloId", req.Id).Error(fmt.Sprintf(
				"Unable to delete SLO when updating between clusters :  %v",
				err))
		}
	}
	for _, zipped := range openSpecServices {
		// don't need creation metadata
		_, err := applyMonitoringSLODownstream(*zipped.Spec, zipped.Service,
			req.Id, s.p, createReq, s.ctx, s.lg)

		if err != nil {
			anyError = err
		}
	}
	return req, anyError
}

func (s SLOMonitoring) Delete(existing *sloapi.SLOData) error {
	id, clusterId := existing.Id, existing.Service.ClusterId
	err := deleteCortexSLORules(s.p, id, clusterId, s.ctx, s.lg)
	return err
}

func (s SLOMonitoring) Clone(clone *sloapi.SLOData) (string, error) {
	var anyError error
	createdSlos, err := s.p.CreateSLO(s.ctx, &sloapi.CreateSLORequest{
		SLO:      clone.SLO,
		Services: []*sloapi.Service{clone.Service},
	})
	if err != nil {
		return "", err
	}
	// should only create one slo
	if len(createdSlos.Items) > 1 {
		anyError = status.Error(codes.Internal, "Created more than one SLO")
	}
	clone.Id = createdSlos.Items[0].Id
	return clone.Id, anyError
}

// Status Only return errors here that should be considered severe InternalServerErrors
//
// - First Checks if it has NoData
// - If it has Data, check if it is within budget
// - If is within budget, check if any alerts are firing
func (s SLOMonitoring) Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error) {
	curState := sloapi.SLOStatusState_Ok

	// check if the recording rule has data
	// rrecording, := existing.Id + RecordingRuleSuffix
	rresp, err := s.p.adminClient.Get().Query(
		s.ctx,
		&cortexadmin.QueryRequest{
			Tenants: []string{existing.Service.ClusterId},
			Query:   fmt.Sprintf(`slo:sli_error:ratio_rate5m{%s="%s"}`, sloOpniIdLabel, existing.Id),
		},
	)
	if err != nil {
		s.lg.Error(fmt.Sprintf("Status : Got error for recording rule %v", err))
		return nil, err
	}
	q, err := unmarshal.UnmarshalPrometheusResponse(rresp.Data)
	if err != nil {
		s.lg.Error(fmt.Sprintf("%v", err))
		return nil, err
	}
	switch q.V.Type() {
	case model.ValVector:
		vv := q.V.(model.Vector)
		if len(vv) == 0 {
			curState = sloapi.SLOStatusState_NoData
		} else {
			curState = sloapi.SLOStatusState_Ok
		}
	default: //FIXME: For now, return internal errors if we can't match result to a vector result
		s.lg.Error(fmt.Sprintf("Unexpected response type '%v' from Prometheus for recording rule", q.V.Type()))
		return &sloapi.SLOStatus{
			State: sloapi.SLOStatusState_InternalError,
		}, nil

	}
	// Check if the metadata rules show we have breached the budget
	// metadataRuleId := existing.Id + MetadataRuleSuffix
	if curState == sloapi.SLOStatusState_Ok {
		_, err := s.p.adminClient.Get().Query(
			s.ctx,
			&cortexadmin.QueryRequest{
				Tenants: []string{existing.Service.ClusterId},
				Query:   "", // TODO : meaningful metadata queries here
			},
		)
		if err != nil {
			return &sloapi.SLOStatus{
				State: sloapi.SLOStatusState_InternalError,
			}, nil
		}
		// TODO : evaluate metadata rules
	}

	if curState == sloapi.SLOStatusState_Ok {
		// Check if the conditions of any of the alerting rules are met
		// alertRuleId := existing.Id + AlertRuleSuffix
		_, err := s.p.adminClient.Get().Query(
			s.ctx,
			&cortexadmin.QueryRequest{
				Tenants: []string{existing.Service.ClusterId},
				Query:   "", // TODO : meaningful query to check alerting conditions here
			},
		)
		if err != nil {
			s.lg.Error(fmt.Sprintf("Status : Got error for recording rule %v", err))
		}
	}
	return &sloapi.SLOStatus{
		State: curState,
	}, nil
}

func (m *MonitoringServiceBackend) WithCurrentRequest(req proto.Message, ctx context.Context) ServiceBackend {
	m.req = req
	m.ctx = ctx
	return m
}

func (m MonitoringServiceBackend) List(clusters *corev1.ClusterList) (*sloapi.ServiceList, error) {
	res := &sloapi.ServiceList{}
	var cl []string
	for _, c := range clusters.Items {
		cl = append(cl, c.Id)
		m.lg.Debug("Found cluster with id %v", c.Id)
	}
	discoveryQuery := `group by(job)({__name__!=""})`

	for _, c := range clusters.Items {
		resp, err := m.p.adminClient.Get().Query(m.ctx, &cortexadmin.QueryRequest{
			Tenants: []string{c.Id},
			Query:   discoveryQuery,
		})
		if err != nil {
			m.lg.Error(fmt.Sprintf("Failed to query cluster %v: %v", c.Id, err))
			return nil, err
		}
		data := resp.GetData()
		m.lg.Debug(fmt.Sprintf("Received service data:\n %s from cluster %s ", string(data), c.Id))
		q, err := unmarshal.UnmarshalPrometheusResponse(data)
		switch q.V.Type() {
		case model.ValVector:
			vv := q.V.(model.Vector)
			for _, v := range vv {

				res.Items = append(res.Items, &sloapi.Service{
					JobId:     string(v.Metric["job"]),
					ClusterId: c.Id,
				})
			}

		}
	}
	return res, nil
}

func (m MonitoringServiceBackend) GetMetricId() (*MetricIds, error) {
	req := m.req.(*sloapi.MetricRequest)
	goodMetricId, totalMetricId, err := assignMetricToJobId(m.p, m.ctx, req)
	if err != nil {
		m.lg.Error(fmt.Sprintf("Unable to assign metric to job: %v", err))
		return nil, err
	}
	return &MetricIds{
		Good:  goodMetricId,
		Total: totalMetricId,
	}, nil
}
