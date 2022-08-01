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

func setEndpointImplementationIfAvailable(p *Plugin, ctx context.Context, req *alertingv1alpha.AlertCondition, newId string) error {
	if req.NotificationId != nil { // create the endpoint implementation
		if req.Details == nil {
			return validation.Error("alerting notification details must be set if you specify a notification target")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			return err
		}

		_, err := p.CreateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			NotificationId: &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: newId},
			Implementation: req.Details,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func updateEndpointImplemetation(p *Plugin, ctx context.Context, req *alertingv1alpha.AlertCondition, id string) error {
	if req.NotificationId != nil { // create the endpoint implementation
		if req.Details == nil {
			return validation.Error("alerting notification details must be set")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			return err
		}

		_, err := p.UpdateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			NotificationId: &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: id},
			Implementation: req.Details,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func handleUpdateEndpointImplementation(p *Plugin,
	ctx context.Context,
	id string,
	existing *alertingv1alpha.AlertCondition,
	new *alertingv1alpha.AlertCondition) error {
	if existing.NotificationId == nil { // no implementation previously set
		return setEndpointImplementationIfAvailable(p, ctx, new, id)
	} else if new.NotificationId == nil { // delete implementation
		_, err := p.DeleteEndpointImplementation(ctx, &corev1.Reference{Id: *existing.NotificationId})
		return err
	} else { // both are set update
		return updateEndpointImplemetation(p, ctx, new, id)
	}
}

func setupCondition(ctx context.Context, req *alertingv1alpha.AlertCondition, newId string) (*corev1.Reference, error) {
	switch req.AlertType {
	case &alertingv1alpha.AlertCondition_System{}: // opni system evaluates these conditions elsewhere in the code
		return &corev1.Reference{Id: newId}, nil
	default:
		return nil, shared.AlertingErrNotImplemented
	}
}

func deleteCondition(ctx context.Context, req *alertingv1alpha.AlertCondition, id string) error {
	switch req.AlertType {
	case &alertingv1alpha.AlertCondition_System{}: // opni system evaluates these conditions elsewhere in the code
		return validation.Error("User should not be able to delete system alert conditions")
	default:
		return shared.AlertingErrNotImplemented
	}
}

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*corev1.Reference, error) {
	newId := uuid.New().String()
	if err := p.storage.Get().Conditions.Put(ctx, path.Join(conditionPrefix, newId), req); err != nil {
		return nil, err
	}
	if err := setEndpointImplementationIfAvailable(p, ctx, req, newId); err != nil {
		return nil, err
	}

	return setupCondition(ctx, req, newId)
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
