package alerting

import (
	"context"
	"path"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const conditionPrefix = "/alerting/conditions"

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*corev1.Reference, error) {
	newId := uuid.New().String()
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, newId), req); err != nil {
		return nil, err
	}
	// TODO check for alert type
	if req.NotificationId != nil { // create the endpoint implementation
		if req.Details == nil {
			return nil, validation.Error("alerting notification details must be set if you specify a notification target")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			return nil, err
		}

		_, err := p.CreateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			NotificationId: &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: newId},
			Implementation: req.Details,
		})
		if err != nil {
			return nil, err
		}
	}

	switch req.AlertType {
	case &alertingv1alpha.AlertCondition_System{}: // opni system evaluates these conditions elsewhere in the code
		return &corev1.Reference{Id: newId}, nil
	default:
		return nil, shared.AlertingErrNotImplemented
	}
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error) {
	return p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, ref.Id))
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error) {
	items, err := list(ctx, p.storage.Get().Conditions, conditionPrefix)
	if err != nil {
		return nil, err
	}
	return &alertingv1alpha.AlertConditionList{Items: items}, nil
}

// req.Id is the condition id reference
func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().Conditions.Get(ctx, path.Join(conditionPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	overrideLabels := req.UpdateAlert.Labels

	// TODO  check if a previous notification implementation exists

	// sub TODO check if we are removing it by passing no implementation

	// sub TODO check if we are adding a new implementation from scratch

	// default case : updating an existing implementation
	if req.UpdateAlert.NotificationId != nil { // create the endpoint implementation
		if req.UpdateAlert.Details == nil {
			return nil, validation.Error("alerting notification details must be set")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.UpdateAlert.NotificationId}); err != nil {
			return nil, err
		}

		_, err := p.UpdateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			NotificationId: &corev1.Reference{Id: *req.UpdateAlert.NotificationId},
			ConditionId:    &corev1.Reference{Id: req.Id.Id},
			Implementation: req.UpdateAlert.Details,
		})
		if err != nil {
			return nil, err
		}
	}

	// TODO same as above

	// if same type, naive update

	// if different type, need to delete the old condition checker and create a new one

	switch req.UpdateAlert.AlertType {
	case &alertingv1alpha.AlertCondition_System{}: // opni system evaluates these conditions elsewhere in the code
		//
	default:
		//
	}

	proto.Merge(existing, req.UpdateAlert)
	existing.Labels = overrideLabels
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, req.Id.Id), existing); err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if err := p.storage.Get().AlertEndpoint.Delete(ctx, path.Join(endpointPrefix, ref.Id)); err != nil {
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
