package alerting

import (
	"context"
	"path"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const endpointPrefix = "/alerting/endpoints"

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error) {
	newId := uuid.New().String()
	if err := p.storage.Get().AlertEndpoint.Put(path.Join(endpointPrefix, newId), req); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	return p.storage.Get().AlertEndpoint.Get(path.Join(endpointPrefix, ref.Id))
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().AlertEndpoint.Get(path.Join(endpointPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	proto.Merge(existing, req.UpdateAlert)
	if err := p.storage.Get().AlertEndpoint.Put(path.Join(endpointPrefix, req.Id.Id), existing); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context, req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	items, err := list(p.storage.Get().AlertEndpoint, endpointPrefix)
	if err != nil {
		return nil, err
	}
	return &alertingv1alpha.AlertEndpointList{Items: items}, nil
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if err := p.storage.Get().AlertEndpoint.Delete(path.Join(endpointPrefix, ref.Id)); err != nil {
		return nil, err
	}

	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	// Create Endpoint

	// Trigger it using httpv2 api

	// Delete Endpoint
	return nil, shared.AlertingErrNotImplemented
}
