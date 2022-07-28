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

const conditionPrefix = "/alerting/conditions"

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*emptypb.Empty, error) {
	newId := uuid.New().String()
	if err := p.storage.Get().Conditions.Put(path.Join(conditionPrefix, newId), req); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error) {
	return p.storage.Get().Conditions.Get(path.Join(conditionPrefix, ref.Id))
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error) {
	items, err := list(p.storage.Get().Conditions, conditionPrefix)
	if err != nil {
		return nil, err
	}
	return &alertingv1alpha.AlertConditionList{Items: items}, nil
}

func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().Conditions.Get(path.Join(conditionPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	proto.Merge(existing, req.UpdateAlert)
	if err := p.storage.Get().Conditions.Put(path.Join(conditionPrefix, req.Id.Id), existing); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if err := p.storage.Get().AlertEndpoint.Delete(path.Join(endpointPrefix, ref.Id)); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) PreviewAlertCondition(ctx context.Context, req *alertingv1alpha.PreviewAlertConditionRequest) (*alertingv1alpha.PreviewAlertConditionResponse, error) {
	// Create alert condition

	// measure status

	// Delete alert condition

	// return status
	return nil, shared.AlertingErrNotImplemented
}
