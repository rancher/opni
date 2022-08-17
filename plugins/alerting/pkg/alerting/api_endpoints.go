package alerting

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/structpb"
	"path"

	"github.com/google/uuid"
	cfg "github.com/prometheus/alertmanager/config"
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

func validateSlack(v *alertingv1alpha.SlackEndpoint) error {
	if v == nil {
		return validation.Error("Missing required field: slack endpoint")
	}
	_, err := NewSlackReceiver("id not used", v)
	return err
}

func validateEmail(v *alertingv1alpha.EmailEndpoint) error {
	if v == nil {
		return validation.Error("Must pass in a non-nil email endpoint")
	}
	_, err := NewEmailReceiver("id not used", v)
	return err
}

func handleAlertEndpointValidation(ctx context.Context, req *alertingv1alpha.AlertEndpoint) error {
	if req == nil {
		return fmt.Errorf("must pass in a non-nil alert endpoint")
	}
	if s := req.GetSlack(); s != nil {
		return validateSlack(req.GetSlack())
	}
	if e := req.GetEmail(); e != nil {
		return validateEmail(req.GetEmail())
	}
	return validation.Error("Unhandled endpoint/implementation details")
}

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error) {
	newId := uuid.New().String()
	if err := handleAlertEndpointValidation(ctx, req); err != nil {
		return nil, err
	}
	if err := p.storage.Get().AlertEndpoint.Put(ctx, path.Join(endpointPrefix, newId), req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	return p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, ref.Id))
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	_, err := p.storage.Get().AlertEndpoint.Get(ctx, path.Join(endpointPrefix, req.Id.Id))
	if err != nil {
		return nil, err
	}
	if err := handleAlertEndpointValidation(ctx, req.UpdateAlert); err != nil {
		return nil, err
	}
	if err := p.storage.Get().AlertEndpoint.Put(ctx, path.Join(endpointPrefix, req.Id.Id), req.UpdateAlert); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context,
	req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	ids, endpoints, err := listWithKeys(ctx, p.storage.Get().AlertEndpoint, endpointPrefix)
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
	if err := p.storage.Get().AlertEndpoint.Delete(ctx, path.Join(endpointPrefix, ref.Id)); err != nil {
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

func processEndpointDetails(conditionId string, req *alertingv1alpha.CreateImplementation, endpointDetails *alertingv1alpha.AlertEndpoint) (*cfg.Receiver, error) {
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
	// newId := uuid.New().String()
	backend := p.endpointBackend.Get()
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
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from UpdateAlertCondition
func (p *Plugin) UpdateEndpointImplementation(ctx context.Context, req *alertingv1alpha.CreateImplementation) (*emptypb.Empty, error) {
	backend := p.endpointBackend.Get()
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
	config, err := configFromBackend(backend, ctx, p)
	if err != nil {
		return nil, err
	}
	if err := config.DeleteReceiver(req.Id); err != nil {
		return nil, err
	}
	err = applyConfigToBackend(backend, ctx, p, config)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// HandleCortexWebhook
//
// cortex rules created with opni-alerting need to integrate with
// opni-alerting's receivers.
// It interacts with opni-alerting's receivers by calling this webhook
//
//
// Data passed into this function is in the form :
//The Alertmanager will send HTTP POST requests in the following JSON format to the configured endpoint:
//```json
//{
//	"version": "4",
//	"groupKey": <string>,              // key identifying the group of alerts (e.g. to deduplicate)
//	"truncatedAlerts": <int>,          // how many alerts have been truncated due to "max_alerts"
//	"status": "<resolved|firing>",
//	"receiver": <string>,
//	"groupLabels": <object>,
//	"commonLabels": <object>,
//	"commonAnnotations": <object>,
//	"externalURL": <string>,           // backlink to the Alertmanager.
//	"alerts": [
//	{
//	"status": "<resolved|firing>",
//	"labels": <object>,
//	"annotations": <object>,
//	"startsAt": "<rfc3339>",
//	"endsAt": "<rfc3339>",
//	"generatorURL": <string>,      // identifies the entity that caused the alert
//	"fingerprint": <string>        // fingerprint to identify the alert
//	},
//...
//]
//}
//````
//
func (p *Plugin) HandleCortexWebhook(ctx context.Context, s *structpb.Struct) (*emptypb.Empty, error) {
	//TODO implement me

	// parse all the annotations from the cortex alert

	// parse the condition id from the annotations

	// call p.TriggerAlerts(ctx, conditionId, annotations)

	return nil, shared.AlertingErrNotImplemented
}
