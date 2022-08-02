package alerting

import (
	"context"
	"fmt"
	"path"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const conditionPrefix = "/alerting/conditions"

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*corev1.Reference, error) {
	newId := uuid.New().String()
	_, err := setupCondition(ctx, req, newId)
	if err != nil {
		return nil, err
	}
	if err := setEndpointImplementationIfAvailable(p, ctx, req, newId); err != nil {
		return nil, err
	}
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, newId), req); err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: newId}, nil
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error) {
	return p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, ref.Id))
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error) {
	keys, items, err := listWithKeys(ctx, p.storage.Get().Conditions, conditionPrefix)
	if err != nil {
		return nil, err
	}
	if len(keys) != len(items) {
		return nil, fmt.Errorf("Internal error : mismatched number of keys")
	}
	res := &alertingv1alpha.AlertConditionList{}
	for i := range keys {
		res.Items = append(res.Items, &alertingv1alpha.AlertConditionWithId{
			Id:             &corev1.Reference{Id: keys[i]},
			AlertCondition: items[i],
		})
	}
	return res, nil
}

// req.Id is the condition id reference
func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	overrideLabels := req.UpdateAlert.Labels

	if err := handleUpdateEndpointImplementation(p, ctx, req.Id.Id, existing, req.UpdateAlert); err != nil {
		return nil, err
	}
	// UPDATE THE ACTUAL CONDITION
	// until we have a more complicated setup, deleting then recreating is fine
	p.DeleteAlertCondition(ctx, &corev1.Reference{Id: req.Id.Id})
	_, err = setupCondition(ctx, req.UpdateAlert, req.Id.Id)
	if err != nil {
		return nil, err
	}

	proto.Merge(existing, req.UpdateAlert)
	existing.Labels = overrideLabels
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, req.Id.Id), existing); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, ref.Id))
	if err != nil {
		return nil, err
	}
	if err := deleteCondition(ctx, existing, ref.Id); err != nil {
		return nil, err
	}
	_, err = p.DeleteEndpointImplementation(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) PreviewAlertCondition(ctx context.Context,
	req *alertingv1alpha.PreviewAlertConditionRequest) (*alertingv1alpha.PreviewAlertConditionResponse, error) {
	// Create alert condition

	// measure status

	// Delete alert condition

	// return status
	return nil, shared.AlertingErrNotImplemented
}
