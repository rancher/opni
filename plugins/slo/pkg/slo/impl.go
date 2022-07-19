package slo

import (
	"context"
	"fmt"
	"path"

	v1 "github.com/alexandreLamarre/oslo/pkg/manifest/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	sloapi "github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func (s SLOMonitoring) WithCurrentRequest(req proto.Message, ctx context.Context) SLOStore {
	s.req = req
	s.ctx = ctx
	return s
}

// OsloSpecs ----> sloth IR ---> Prometheus SLO --> Cortex Rule groups
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
		createdSlos, err := applyMonitoringSLODownstream(*zipped.Spec, zipped.Service, "", s.p, req, s.ctx, s.lg)

		if err != nil {
			anyError = err
		}
		for _, data := range createdSlos {
			returnedSloId.Items = append(returnedSloId.Items, &corev1.Reference{Id: data.Id})
			if err := s.p.storage.Get().SLOs.Put(path.Join("/slos", data.Id), data); err != nil {
				return nil, err
			}
			if err != nil {
				anyError = err
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
		s.p.DeleteSLO(s.ctx, &corev1.Reference{Id: req.Id})
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
	err := deleteCortexSLORules(s.p, existing, s.ctx, s.lg)
	return err
}

func (s SLOMonitoring) Clone(clone *sloapi.SLOData) (string, error) {
	var anyError error
	createdSlos, err := s.p.CreateSLO(s.ctx, &sloapi.CreateSLORequest{
		SLO:      clone.SLO,
		Services: []*sloapi.Service{clone.Service},
	})
	if err != nil {
		anyError = err
	}
	// should only create one slo
	if len(createdSlos.Items) > 1 {
		anyError = status.Error(codes.Internal, "Created more than one SLO")
	}
	clone.Id = createdSlos.Items[0].Id
	return clone.Id, anyError
}

// Only return errors here that should be considered servere InternalServerErrors
func (s SLOMonitoring) Status(existing *sloapi.SLOData) (*sloapi.SLOStatus, error) {
	defaultState := sloapi.SLOStatusState_NoData

	// check if the recording rule has data
	recordingRuleId := existing.Id + RecordingRuleSuffix
	rresp, err := s.p.adminClient.Get().Query(
		s.ctx,
		&cortexadmin.QueryRequest{
			Tenants: []string{existing.Service.ClusterId},
			Query:   recordingRuleId, // TODO : meaningful query to most recent data
		},
	)
	if err != nil {
		s.lg.Error(fmt.Sprintf("Status : Got error for recording rule %v", err))
	} else {
		s.lg.Debug("%v", rresp)
	}
	// Check if the metadata rules show we have breached the budget
	metadataRuleId := existing.Id + MetadataRuleSuffix
	mresp, err := s.p.adminClient.Get().Query(
		s.ctx,
		&cortexadmin.QueryRequest{
			Tenants: []string{existing.Service.ClusterId},
			Query:   metadataRuleId, // TODO : meaningful query to error budget metadata here
		},
	)
	if err != nil {
		s.lg.Error(fmt.Sprintf("Status : Got error for recording rule %v", err))
	} else {
		s.lg.Debug("%v", mresp)
	}
	// Check if the conditions of any of the alerting rules are met
	alertRuleId := existing.Id + AlertRuleSuffix
	aresp, err := s.p.adminClient.Get().Query(
		s.ctx,
		&cortexadmin.QueryRequest{
			Tenants: []string{existing.Service.ClusterId},
			Query:   alertRuleId, // TODO : meaningful query to check alerting conditions here
		},
	)
	if err != nil {
		s.lg.Error(fmt.Sprintf("Status : Got error for recording rule %v", err))
	} else {
		s.lg.Debug("%v", aresp)
	}
	return &sloapi.SLOStatus{
		State: defaultState,
	}, nil
}
