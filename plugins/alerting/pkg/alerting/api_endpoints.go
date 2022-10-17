package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1alpha.AlertEndpoint) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	newId := uuid.New().String()
	if err := p.storageNode.CreateEndpointsStorage(ctx, newId, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertEndpoint, error) {
	return p.storageNode.GetEndpointStorage(ctx, ref.Id)
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1alpha.UpdateAlertEndpointRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	// List relationships
	allRelationships, err := p.ListRoutingRelationships(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	// if the endpoint is involved in any conditions
	// get conditions involved, get their endpoint details
	involvedConditions := allRelationships.InvolvedConditionsForEndpoint(req.Id.Id)
	if len(involvedConditions) > 0 {
		_, err := p.UpdateIndividualEndpointInRoutingNode(ctx, &alertingv1alpha.FullAttachedEndpoint{
			EndpointId:    req.Id.Id,
			AlertEndpoint: req.UpdateAlert,
		})
		if err != nil {
			return nil, err
		}
	}
	if err := p.storageNode.UpdateEndpointStorage(ctx, req.Id.Id, req.UpdateAlert); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context,
	req *alertingv1alpha.ListAlertEndpointsRequest) (*alertingv1alpha.AlertEndpointList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ids, endpoints, err := p.storageNode.ListWithKeyEndpointStorage(ctx)
	if err != nil {
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
	if err := p.storageNode.DeleteEndpointStorage(ctx, ref.Id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1alpha.TestAlertEndpointRequest) (*alertingv1alpha.TestAlertEndpointResponse, error) {
	lg := p.Logger.With("Handler", "TestAlertEndpoint")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	// - Create dummy Endpoint id
	dummyConditionId := "test-" + uuid.New().String()
	dummyEndpointId := "test-" + uuid.New().String()
	lg.Debugf("dummy id : %s", dummyConditionId)
	var typeName string
	if s := req.Endpoint.GetSlack(); s != nil {
		typeName = "slack"
	}
	if e := req.Endpoint.GetEmail(); e != nil {
		typeName = "email"
	}
	if typeName == "" {
		typeName = "unknwon"
	}

	createImpl := &alertingv1alpha.RoutingNode{
		ConditionId: &corev1.Reference{Id: dummyConditionId}, // is used as a unique identifier
		FullAttachedEndpoints: &alertingv1alpha.FullAttachedEndpoints{
			InitialDelay: durationpb.New(time.Duration(time.Second * 0)),
			Items: []*alertingv1alpha.FullAttachedEndpoint{
				{
					EndpointId:    dummyEndpointId,
					AlertEndpoint: req.Endpoint,
				},
			},
			Details: &alertingv1alpha.EndpointImplementation{
				Title: fmt.Sprintf("Test %s Alert (%s)", typeName, time.Now().Format("2006-01-02T15:04:05 -07:00:00")),
				Body:  "Opni-alerting is sending you a test alert",
			},
		},
	}
	// create condition routing node
	_, err := p.CreateConditionRoutingNode(ctx, createImpl)
	if err != nil {
		return nil, err
	}
	defer func() {
		// FIXME: retrier backoff?
		go func() {
			_, err := p.DeleteConditionRoutingNode(ctx, &corev1.Reference{Id: dummyConditionId})
			if err != nil {
				lg.Errorf("failed to delete dummy condition node : %v", err)
			}
		}()

	}()
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		lg.Errorf("Failed to fetch plugin options within timeout : %s", err)
		return nil, err
	}
	var availableEndpoint string
	status, err := p.opsNode.GetClusterConfiguration(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if status.NumReplicas == 1 { // exactly one that is the controller
		availableEndpoint = options.GetControllerEndpoint()
	} else {
		availableEndpoint = options.GetWorkerEndpoint()
	}
	// trigger it
	lg.Debug("active workload endpoint :%s", availableEndpoint)
	alert := &backend.PostableAlert{}
	alert.WithCondition(dummyConditionId)
	var alerts []*backend.PostableAlert
	alerts = append(alerts, alert)
	lg.Debugf("sending alert to alertmanager : %v, %v", alert.Annotations, alert.Labels)
	_, resp, err := backend.PostAlert(context.Background(), availableEndpoint, alerts)
	if err != nil {
		lg.Errorf("Error while posting alert : %s", err)
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, shared.WithInternalServerErrorf("failed to send alert to alertmanager")
	}
	lg.Debugf("Got response %s from alertmanager", resp.Status)
	return &alertingv1alpha.TestAlertEndpointResponse{}, nil
}

func (p *Plugin) CreateConditionRoutingNode(ctx context.Context, req *alertingv1alpha.RoutingNode) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	amConfig, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	err = amConfig.CreateRoutingNodeForCondition(
		req.ConditionId.Id,
		req.GetFullAttachedEndpoints(),
		internalConfig,
	)
	if err != nil {
		return nil, err
	}
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) UpdateConditionRoutingNode(ctx context.Context, req *alertingv1alpha.RoutingNode) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	amConfig, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	err = amConfig.UpdateRoutingNodeForCondition(
		req.ConditionId.Id,
		req.GetFullAttachedEndpoints(),
		internalConfig,
	)
	if err != nil {
		return nil, err
	}
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Id must be a conditionId
func (p *Plugin) DeleteConditionRoutingNode(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	amConfig, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	err = amConfig.DeleteRoutingNodeForCondition(req.Id, internalConfig)
	if err != nil {
		return nil, err
	}
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListRoutingRelationships(ctx context.Context, _ *emptypb.Empty) (*alertingv1alpha.RoutingRelationships, error) {
	_, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	conditions := make(map[string]*alertingv1alpha.EndpointRoutingMap)
	for conditionId, endpointMap := range internalConfig.Content {
		conditions[conditionId] = &alertingv1alpha.EndpointRoutingMap{
			Endpoints: make(map[string]*alertingv1alpha.EndpointMetadata),
		}
		for endpointId, metadata := range endpointMap {
			conditions[conditionId].Endpoints[endpointId] = &alertingv1alpha.EndpointMetadata{
				Position:     int32(*metadata.Position),
				EndpointType: metadata.EndpointType,
			}
		}
	}

	return &alertingv1alpha.RoutingRelationships{
		Conditions: conditions,
	}, nil
}

func (p *Plugin) UpdateIndividualEndpointInRoutingNode(ctx context.Context, attachedEndpoint *alertingv1alpha.FullAttachedEndpoint) (*emptypb.Empty, error) {
	amConfig, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	err = amConfig.UpdateIndividualEndpointNode(attachedEndpoint, internalConfig)
	if err != nil {
		return nil, err
	}
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// FIXME: errors in this function can result in mismatched information
func (p *Plugin) DeleteIndividualEndpointInRoutingNode(ctx context.Context, reference *corev1.Reference) (*emptypb.Empty, error) {
	lg := p.Logger.With("action", "DeleteIndividualEndpointInRoutingNode")
	deleteEndpointId := reference.Id
	amConfig, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	internalConfigCopy, err := internalConfig.DeepCopy()
	if err != nil {
		return nil, err
	}

	_, err = amConfig.DeleteIndividualEndpointNode(deleteEndpointId, internalConfig)
	if err != nil {
		return nil, err
	}

	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
	if err != nil {
		return nil, err
	}

	involvedConditions := []string{}
	for conditionId, endpointMap := range internalConfigCopy.Content {
		for endpointId := range endpointMap {
			if endpointId == deleteEndpointId {
				involvedConditions = append(involvedConditions, conditionId)
			}
		}
	}

	wg := sync.WaitGroup{}
	for _, conditionId := range involvedConditions {
		conditionId := conditionId
		wg.Add(1)
		go func() {
			defer wg.Done()
			alertCondition, err := p.GetAlertCondition(ctx, &corev1.Reference{
				Id: conditionId,
			})
			if err != nil { //FIXME: should flag all subsequent errors as a hard reset to the AM/ internal config
				lg.Error(err)
				return
			}
			var idxToDelete int
			for idx, endpoint := range alertCondition.AttachedEndpoints.Items {
				if endpoint.EndpointId == deleteEndpointId {
					idxToDelete = idx
					break
				}
			}
			alertCondition.AttachedEndpoints.Items = slices.Delete(
				alertCondition.AttachedEndpoints.Items,
				idxToDelete,
				idxToDelete+1,
			)
			_, err = p.UpdateAlertCondition(ctx, &alertingv1alpha.UpdateAlertConditionRequest{
				Id: &corev1.Reference{
					Id: conditionId,
				},
				UpdateAlert: alertCondition,
			})
			if err != nil {
				lg.Error(err)
				return
			}
		}()
	}

	wg.Wait()

	return &emptypb.Empty{}, nil
}
