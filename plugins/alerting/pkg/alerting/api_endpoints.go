package alerting

import (
	"context"
	"path"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/emptypb"
)

const endpointPrefix = "/alerting/endpoints"

func configFromBackend(backend RuntimeEndpointBackend, ctx context.Context, p *Plugin) (*ConfigMapData, error) {
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
	err := backend.Put(ctx, p, "alertmanager.yaml", config)
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
	if err := req.Validate(); err != nil {
		return nil, err
	}
	newId := uuid.New().String()
	if err := p.storage.Get().AlertEndpoint.Put(ctx, path.Join(endpointPrefix, newId), req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	ctx, ca := setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	defer ca()
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	return storage.AlertEndpoint.Get(ctx, path.Join(endpointPrefix, ref.Id))
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, ca := setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	defer ca()
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	_, err = storage.AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	if err := storage.AlertEndpoint.Put(ctx, path.Join(endpointPrefix, req.Id.Id), req.UpdateAlert); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context,
	req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, ca := setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	defer ca()
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	ids, endpoints, err := listWithKeys(ctx, storage.AlertEndpoint, endpointPrefix)
	if err != nil {
		return nil, err
	}
	if len(ids) != len(endpoints) {
		return nil, err
	}
	items := []*alertingv1alpha.AlertEndpointWithId{}
	for idx := range ids {
		items = append(items, &alertingv1alpha.AlertEndpointWithId{
			Id:       &corev1.Reference{Id: ids[idx]},
			Endpoint: endpoints[idx],
		})
	}
	return &alertingv1alpha.AlertEndpointList{Items: items}, nil
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	ctx, ca := setPluginHandlerTimeout(ctx, time.Duration(time.Second*10))
	defer ca()
	storage, err := p.storage.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := storage.AlertEndpoint.Delete(ctx, path.Join(endpointPrefix, ref.Id)); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// TODO
func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	// - Create Endpoint
	// - Trigger it using httpv2 api
	// - Delete Endpoint
	return nil, shared.AlertingErrNotImplemented
}

func processEndpointDetails(conditionId string, req *alertingv1alpha.CreateImplementation, endpointDetails *alertingv1alpha.AlertEndpoint) (*Receiver, error) {
	if s := endpointDetails.GetSlack(); s != nil {
		recv, err := NewSlackReceiver(conditionId, s)
		if err != nil {
			return nil, err // some validation error
		}
		recv, err = WithSlackImplementation(recv, req.Implementation)
		if err != nil {
			return nil, err
		}
		return recv, nil
	}
	if e := endpointDetails.GetEmail(); e != nil {
		recv, err := NewEmailReceiver(conditionId, e)
		if err != nil {
			return nil, err // some validation error
		}
		recv, err = WithEmailImplementation(recv, req.Implementation)
		if err != nil {
			return nil, err
		}
		return recv, nil
	}
	return nil, validation.Error("Unhandled endpoint/implementation details")
}

// Called from CreateAlertCondition
func (p *Plugin) CreateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, ca := setPluginHandlerTimeout(ctx, time.Duration(time.Second*30))
	defer ca()
	// newId := uuid.New().String()
	backend, err := p.endpointBackend.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	config, err := configFromBackend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.EndpointId.Id))
	if err != nil {
		return nil, err
	}
	conditionId := req.ConditionId.Id
	recv, err := processEndpointDetails(conditionId, req, endpointDetails)
	if err != nil {
		return nil, err
	}
	config.AppendReceiver(recv)
	config.AppendRoute(recv, req)
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from UpdateAlertCondition
func (p *Plugin) UpdateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ctx, ca := setPluginHandlerTimeout(ctx, time.Duration(time.Second*30))
	defer ca()
	backend, err := p.endpointBackend.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	config, err := configFromBackend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.EndpointId.Id))
	if err != nil {
		return nil, err
	}
	conditionId := req.ConditionId.Id
	recv, err := processEndpointDetails(conditionId, req, endpointDetails)
	if err != nil {
		return nil, err
	}
	//note: don't need to update routes after this since they hold a ref to the original condition id
	err = config.UpdateReceiver(conditionId, recv)
	if err != nil {
		return nil, err
	}
	err = config.UpdateRoute(conditionId, recv, req)
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
	ctx, ca := setPluginHandlerTimeout(ctx, time.Duration(time.Second*30))
	defer ca()
	backend, err := p.endpointBackend.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	config, err := configFromBackend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	if err := config.DeleteReceiver(req.Id); err != nil {
		return nil, err
	}
	if err := config.DeleteRoute(req.Id); err != nil {
		return nil, err
	}
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
