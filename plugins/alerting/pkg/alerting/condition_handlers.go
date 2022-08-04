/*
  - Functions that handle each endpoint implementation update case
  - Functions that handle each alert condition case
*/
package alerting

import (
	"context"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
)

func setEndpointImplementationIfAvailable(p *Plugin, ctx context.Context, req *alertingv1alpha.AlertCondition, newId string) error {
	if req.NotificationId != nil { // create the endpoint implementation
		if req.Details == nil {
			return validation.Error("alerting notification details must be set if you specify a notification target")
		}
		if _, err := p.GetAlertEndpoint(ctx, &corev1.Reference{Id: *req.NotificationId}); err != nil {
			return err
		}

		_, err := p.CreateEndpointImplementation(ctx, &alertingv1alpha.CreateImplementation{
			EndpointId:     &corev1.Reference{Id: *req.NotificationId},
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
			EndpointId:     &corev1.Reference{Id: *req.NotificationId},
			ConditionId:    &corev1.Reference{Id: id},
			Implementation: req.Details,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func handleUpdateEndpointImplementation(
	p *Plugin,
	ctx context.Context,
	id string,
	existing *alertingv1alpha.AlertCondition,
	new *alertingv1alpha.AlertCondition,
) error {
	if existing.NotificationId == nil { // no implementation previously set
		return setEndpointImplementationIfAvailable(p, ctx, new, id)
	} else if new.NotificationId == nil { // delete implementation
		_, err := p.DeleteEndpointImplementation(ctx, &corev1.Reference{Id: *existing.NotificationId})
		return err
	}
	return updateEndpointImplemetation(p, ctx, new, id)
}

func setupCondition(ctx context.Context, req *alertingv1alpha.AlertCondition, newId string) (*corev1.Reference, error) {
	if s := req.GetSystem(); s != nil {
		return &corev1.Reference{Id: newId}, nil
	}
	return nil, shared.AlertingErrNotImplemented
}

func deleteCondition(ctx context.Context, req *alertingv1alpha.AlertCondition, id string) error {
	switch req.AlertType {
	case &alertingv1alpha.AlertCondition_System{}: // opni system evaluates these conditions elsewhere in the code
		return validation.Error("User should not be able to delete system alert conditions")
	default:
		return shared.AlertingErrNotImplemented
	}
}
