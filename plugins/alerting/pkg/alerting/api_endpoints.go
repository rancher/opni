package alerting

import (
	"context"
	"path"
	"reflect"

	"github.com/google/uuid"
	cfg "github.com/prometheus/alertmanager/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

const endpointPrefix = "/alerting/endpoints"

func configFromBakend(backend RuntimeEndpointBackend, ctx context.Context, p *Plugin) (*ConfigMapData, error) {
	rawConfig, err := backend.Fetch(ctx, p, "alertmanager.yaml")
	if err != nil {
		return nil, err
	}
	config, err := NewConfigMapDataFrom(rawConfig)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func applyConfigToBackend(backend RuntimeEndpointBackend, ctx context.Context, p *Plugin, config *ConfigMapData) error {
	newRawConfig, err := config.Marshal()
	if err != nil {
		return err
	}
	err = backend.Put(ctx, p, "alertmanager.yaml", string(newRawConfig))
	if err != nil {
		return err
	}
	err = backend.Reload(ctx, p)
	if err != nil {
		return err
	}
	return nil
}

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error) {
	newId := uuid.New().String()
	if err := p.storage.Get().AlertEndpoint.Put(ctx, path.Join(endpointPrefix, newId), req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	return p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, ref.Id))
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	existing, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	proto.Merge(existing, req.UpdateAlert)
	if err := p.storage.Get().AlertEndpoint.Put(ctx, path.Join(endpointPrefix, req.Id.Id), existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context, req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	items, err := list(ctx, p.storage.Get().AlertEndpoint, endpointPrefix)
	if err != nil {
		return nil, err
	}
	return &alertingv1alpha.AlertEndpointList{Items: items}, nil
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if err := p.storage.Get().AlertEndpoint.Delete(ctx, path.Join(endpointPrefix, ref.Id)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

//TODO
func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	// Create Endpoint

	// Trigger it using httpv2 api

	// Delete Endpoint
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) GetImplementationFromEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.EndpointImplementation, error) {

	existing, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, ref.Id))
	if err != nil {
		return nil, err
	}
	res := &alertingv1alpha.EndpointImplementation{}
	switch existing.Endpoint {
	case &alertingv1alpha.AlertEndpoint_Slack{}:
		res.Implementation = &alertingv1alpha.EndpointImplementation_Slack{
			Slack: (&alertingv1alpha.SlackImplementation{}).Defaults(),
		}
	case &alertingv1alpha.AlertEndpoint_Email{}:
		res.Implementation = &alertingv1alpha.EndpointImplementation_Email{
			Email: (&alertingv1alpha.EmailImplementation{}).Defaults(),
		}
	}
	return res, nil
}

func processEndpointDetails(conditionId string, req *alertingv1alpha.CreateImplementation, endpointDetails *alertingv1alpha.AlertEndpoint) (*cfg.Receiver, error) {
	switch endpointDetails.Endpoint {

	case &alertingv1alpha.AlertEndpoint_Slack{}:
		//
		recv, err := NewSlackReceiver(conditionId, endpointDetails.GetSlack())
		if err != nil {
			return nil, err // some validation error
		}
		if reflect.TypeOf(req.Implementation.Implementation) != reflect.TypeOf(&alertingv1alpha.EndpointImplementation_Slack{}) {
			return nil, shared.AlertingErrMismatchedImplementation
		}
		recv, err = WithSlackImplementation(recv, req.Implementation.GetSlack())
		if err != nil {
			return nil, err
		}
		return recv, nil

	case &alertingv1alpha.AlertEndpoint_Email{}:
		//
		recv, err := NewEmailReceiver(conditionId, endpointDetails.GetEmail())
		if err != nil {
			return nil, err // some validation error
		}
		if reflect.TypeOf(req.Implementation.Implementation) != reflect.TypeOf(&alertingv1alpha.EndpointImplementation_Email{}) {
			return nil, shared.AlertingErrMismatchedImplementation
		}
		recv, err = WithEmailImplementation(recv, req.Implementation.GetEmail())
		if err != nil {
			return nil, err
		}
		return recv, nil
	default:
		return nil, validation.Error("Unhandled endpoint/implementation details")
	}

}

// Called from CreateAlertCondition
func (p *Plugin) CreateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error) {
	// newId := uuid.New().String()
	backend := p.endpointBackend.Get()
	config, err := configFromBakend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.NotificationId.Id))
	if err != nil {
		return nil, err
	}

	conditionId := req.ConditionId.Id

	recv, err := processEndpointDetails(conditionId, req, endpointDetails)
	if err != nil {
		return nil, err
	}
	config.AppendReceiver(recv)

	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from UpdateAlertCondition
func (p *Plugin) UpdateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error) {
	backend := p.endpointBackend.Get()
	config, err := configFromBakend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.NotificationId.Id))
	if err != nil {
		return nil, err
	}

	conditionId := req.ConditionId.Id
	recv, err := processEndpointDetails(conditionId, req, endpointDetails)
	if err != nil {
		return nil, err
	}

	err = config.UpdateReceiver(conditionId, recv)
	if err != nil {
		return nil, err
	}

	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from DeleteAlertCondition
// Id must be a conditionId
func (p *Plugin) DeleteEndpointImplementation(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	backend := p.endpointBackend.Get()
	config, err := configFromBakend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	config.DeleteReceiver(req.Id)
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	return nil, shared.AlertingErrNotImplemented
}
