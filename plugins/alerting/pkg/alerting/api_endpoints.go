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

	backend := p.endpointBackend.Get()
	config, err := configFromBakend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	switch req.Endpoint {
	case &alertingv1alpha.AlertEndpoint_Slack{}:
		//
		recv, err := NewSlackReceiver(newId, req.GetSlack())
		if err != nil {
			return nil, err // some validation error
		}
		config.AppendReceiver(recv)

	case &alertingv1alpha.AlertEndpoint_Email{}:
		//
		recv, err := NewEmailReceiver(newId, req.GetEmail())
		if err != nil {
			return nil, err // some validation error
		}
		config.AppendReceiver(recv)
	}
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
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
	overrideLabels := req.UpdateAlert.GetLabels()

	backend := p.endpointBackend.Get()
	config, err := configFromBakend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	switch req.UpdateAlert.Endpoint {
	case &alertingv1alpha.AlertEndpoint_Slack{}:
		//
		recv, err := NewSlackReceiver(req.Id.Id, req.UpdateAlert.GetSlack())
		if err != nil {
			return nil, err // some validation error
		}
		err = config.UpdateReceiver(req.Id.Id, recv)
		if err != nil {
			return nil, err
		}

	case &alertingv1alpha.AlertEndpoint_Email{}:
		//
		recv, err := NewEmailReceiver(req.Id.Id, req.UpdateAlert.GetEmail())
		if err != nil {
			return nil, err // some validation error
		}
		err = config.UpdateReceiver(req.Id.Id, recv)
		if err != nil {
			return nil, err
		}
	}
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	// once everything else succeeds we can update the config the user sees
	proto.Merge(existing, req.UpdateAlert)
	existing.Labels = overrideLabels
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

	backend := p.endpointBackend.Get()
	config, err := configFromBakend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	config.DeleteReceiver(ref.Id)
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
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

func (p *Plugin) CreateEndpointImplementation(ctx context.Context, req *alertingv1alpha.EndpointImplementation) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) UpdateEndpointImplementation(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}

func (p *Plugin) DeleteEndpointImplementation(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	return nil, shared.AlertingErrNotImplemented
}
