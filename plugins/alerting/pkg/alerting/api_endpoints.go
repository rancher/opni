package alerting

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/backend"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting/alertstorage"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

func redactSecrets(endp *alertingv1.AlertEndpoint) {
	endp.RedactSecrets()
}

func unredactSecrets(
	ctx context.Context,
	node *alertstorage.StorageNode,
	endpointId string,
	endp *alertingv1.AlertEndpoint,
) error {
	unredacted, err := node.GetEndpoint(ctx, endpointId)
	if err != nil {
		return err
	}
	endp.UnredactSecrets(unredacted)
	return nil
}

func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1.AlertEndpoint) (*corev1.Reference, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	newId := uuid.New().String()
	if err := p.storageNode.CreateEndpoint(ctx, newId, req); err != nil {
		return nil, err
	}
	return &corev1.Reference{
		Id: newId,
	}, nil
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertEndpoint, error) {
	endp, err := p.storageNode.GetEndpoint(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	// handle secrets
	redactSecrets(endp)
	return endp, nil
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1.UpdateAlertEndpointRequest) (*alertingv1.InvolvedConditions, error) {
	if err := unredactSecrets(ctx, p.storageNode, req.Id.Id, req.GetUpdateAlert()); err != nil {
		return nil, err
	}
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
	refList := []*corev1.Reference{}
	if len(involvedConditions) > 0 {
		for _, condId := range involvedConditions {
			refList = append(refList, &corev1.Reference{Id: condId})
		}
		if !req.ForceUpdate {
			return &alertingv1.InvolvedConditions{
				Items: refList,
			}, nil
		}
		_, err := p.UpdateIndividualEndpointInRoutingNode(ctx, &alertingv1.FullAttachedEndpoint{
			EndpointId:    req.Id.Id,
			AlertEndpoint: req.GetUpdateAlert(),
		})
		if err != nil {
			return nil, err
		}
	}
	if err := p.storageNode.UpdateEndpoint(ctx, req.Id.Id, req.GetUpdateAlert()); err != nil {
		return nil, err
	}
	return &alertingv1.InvolvedConditions{
		Items: refList,
	}, nil
}

func (p *Plugin) adminListAlertEndpoints(
	ctx context.Context,
	req *alertingv1.ListAlertEndpointsRequest,
) (*alertingv1.AlertEndpointList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ids, endpoints, err := p.storageNode.ListWithKeysEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	items := []*alertingv1.AlertEndpointWithId{}
	for idx := range ids {
		endp := endpoints[idx]
		items = append(items, &alertingv1.AlertEndpointWithId{
			Id:       &corev1.Reference{Id: ids[idx]},
			Endpoint: endp,
		})
	}
	return &alertingv1.AlertEndpointList{Items: items}, nil
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context,
	req *alertingv1.ListAlertEndpointsRequest) (*alertingv1.AlertEndpointList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ids, endpoints, err := p.storageNode.ListWithKeysEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	items := []*alertingv1.AlertEndpointWithId{}
	for idx := range ids {
		endp := endpoints[idx]
		redactSecrets(endp)
		items = append(items, &alertingv1.AlertEndpointWithId{
			Id:       &corev1.Reference{Id: ids[idx]},
			Endpoint: endp,
		})
	}
	return &alertingv1.AlertEndpointList{Items: items}, nil
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, req *alertingv1.DeleteAlertEndpointRequest) (*alertingv1.InvolvedConditions, error) {
	_, err := p.GetAlertEndpoint(ctx, req.Id)
	if err != nil {
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
	refList := []*corev1.Reference{}
	if len(involvedConditions) > 0 {
		for _, condId := range involvedConditions {
			refList = append(refList, &corev1.Reference{Id: condId})
		}
		if !req.ForceDelete {
			return &alertingv1.InvolvedConditions{
				Items: refList,
			}, nil
		}
		_, err := p.DeleteIndividualEndpointInRoutingNode(ctx, req.Id)
		if err != nil {
			return nil, err
		}
	}

	if err := p.storageNode.DeleteEndpoint(ctx, req.Id.Id); err != nil {
		return nil, err
	}

	return &alertingv1.InvolvedConditions{
		Items: refList,
	}, nil
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1.TestAlertEndpointRequest) (*alertingv1.TestAlertEndpointResponse, error) {
	lg := p.Logger.With("Handler", "TestAlertEndpoint")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var typeName string
	if s := req.Endpoint.GetSlack(); s != nil {
		typeName = "slack"
	}
	if e := req.Endpoint.GetEmail(); e != nil {
		typeName = "email"
	}
	if pg := req.Endpoint.GetPagerDuty(); pg != nil {
		typeName = "pagerduty"
	}
	if typeName == "" {
		typeName = "unknown"
	}
	details := &alertingv1.EndpointImplementation{
		Title: fmt.Sprintf("Test Alert - %s (%s)", typeName, time.Now().Format("2006-01-02T15:04:05 -07:00:00")),
		Body:  "Opni-alerting is sending you a test alert",
	}
	triggerReq, err := p.EphemeralDispatcher(ctx, &alertingv1.EphemeralDispatcherRequest{
		Prefix:        "test",
		Ttl:           durationpb.New(time.Duration(time.Second * 60)),
		NumDispatches: 10,
		Items:         []*alertingv1.AlertEndpoint{req.Endpoint},
		Details:       details,
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		// FIXME: retrier backoff?
		go func() {
			time.Sleep(time.Second * 30)
			// can't use request context here because it will be cancelled
			_, err := p.DeleteConditionRoutingNode(context.Background(),
				triggerReq.TriggerAlertsRequest.ConditionId)
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
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, options)
	if err != nil {
		return nil, err
	}
	// need to trigger multiple alerts here, a reload can cause a context.Cancel()
	// to any pending post requests, resulting in the alert never being sent
	apiNode := backend.NewAlertManagerPostAlertClient(
		ctx,
		availableEndpoint,
		backend.WithLogger(lg),
		backend.WithPostAlertBody(triggerReq.TriggerAlertsRequest.ConditionId.GetId(), nil),
		backend.WithDefaultRetrier(),
		backend.WithExpectClosure(backend.NewExpectStatusCodes([]int{200, 422})))
	err = apiNode.DoRequest()
	if err != nil {
		lg.Errorf("Failed to post alert to alertmanager : %s", err)
	}

	return &alertingv1.TestAlertEndpointResponse{}, nil
}

func (p *Plugin) CreateConditionRoutingNode(ctx context.Context, req *alertingv1.RoutingNode) (*emptypb.Empty, error) {
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

func (p *Plugin) UpdateConditionRoutingNode(ctx context.Context, req *alertingv1.RoutingNode) (*emptypb.Empty, error) {
	lg := p.Logger.With("handler", "UpdateConditionRoutingNode")
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
		if e, ok := status.FromError(err); ok && e.Code() == codes.NotFound { // something went wrong and receiver/route can't be found
			lg.Debug("update failed due to missing internal configuration, creating new routing node")
			_, err = p.CreateConditionRoutingNode(ctx, req)
			if err != nil {
				p.Logger.Error(err)
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
	if err != nil {
		if e, ok := status.FromError(err); ok && e.Code() == codes.FailedPrecondition { // no changes to apply to k8s objects, force a sync with internal AlertManager config
			lg.Debug("forcing sync of internal routing config, since owned k8s objects are up to date")
			info, err := p.opsNode.Fetch(ctx, &emptypb.Empty{})
			if err != nil {
				return nil, err
			}
			_, err = p.opsNode.Reload(ctx, &alertops.ReloadInfo{
				UpdatedConfig: info.RawAlertManagerConfig,
			})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}

// Id must be a conditionId
func (p *Plugin) DeleteConditionRoutingNode(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	amConfig, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	deleteErr := amConfig.DeleteRoutingNodeForCondition(req.Id, internalConfig)
	if e, ok := status.FromError(deleteErr); ok && e.Code() != codes.NotFound {
		return nil, err
	}
	// if the original indexing failed we want to skip applying no changes to the config
	if deleteErr == nil {
		err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
		if err != nil {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListRoutingRelationships(ctx context.Context, _ *emptypb.Empty) (*alertingv1.RoutingRelationships, error) {
	_, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	conditions := make(map[string]*alertingv1.EndpointRoutingMap)
	for conditionId, endpointMap := range internalConfig.Content {
		conditions[conditionId] = &alertingv1.EndpointRoutingMap{
			Endpoints: make(map[string]*alertingv1.EndpointMetadata),
		}
		for endpointId, metadata := range endpointMap {
			conditions[conditionId].Endpoints[endpointId] = &alertingv1.EndpointMetadata{
				Position:     int32(*metadata.Position),
				EndpointType: metadata.EndpointType,
			}
		}
	}

	return &alertingv1.RoutingRelationships{
		Conditions: conditions,
	}, nil
}

func (p *Plugin) UpdateIndividualEndpointInRoutingNode(ctx context.Context, attachedEndpoint *alertingv1.FullAttachedEndpoint) (*emptypb.Empty, error) {
	lg := p.Logger.With("handler", "UpdateIndividualEndpointInRoutingNode")
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
		if e, ok := status.FromError(err); ok && e.Code() == codes.FailedPrecondition { // no changes to apply to k8s objects, force a sync with internal AlertManager config
			lg.Debug("forcing sync of internal routing config, since owned k8s objects are up to date")
			info, err := p.opsNode.Fetch(ctx, &emptypb.Empty{})
			if err != nil {
				return nil, err
			}
			_, err = p.opsNode.Reload(ctx, &alertops.ReloadInfo{
				UpdatedConfig: info.RawAlertManagerConfig,
			})
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}

// FIXME: errors in this function can result in mismatched information,
// will be fixed when we move to a fully virtualize config
func (p *Plugin) DeleteIndividualEndpointInRoutingNode(ctx context.Context, reference *corev1.Reference) (*emptypb.Empty, error) {
	lg := p.Logger.With("handler", "DeleteIndividualEndpointInRoutingNode")
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

	if e, ok := status.FromError(err); ok && e.Code() != codes.NotFound {
		lg.Error(err)
		return nil, err
	}
	// if condition was never indexed in the internal routing, we need to continue and remove it
	// from all conditions that specify it

	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig)
	if err != nil {
		lg.Error(err)
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
			_, err = p.UpdateAlertCondition(ctx, &alertingv1.UpdateAlertConditionRequest{
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

func (p *Plugin) EphemeralDispatcher(ctx context.Context, req *alertingv1.EphemeralDispatcherRequest) (*alertingv1.EphemeralDispatcherResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ephemeralId := uuid.New().String()
	ephemeralId = req.GetPrefix() + "-" + ephemeralId

	fullAttachEndpoint := []*alertingv1.FullAttachedEndpoint{}
	for _, endp := range req.GetItems() {
		fullAttachEndpoint = append(fullAttachEndpoint, &alertingv1.FullAttachedEndpoint{
			EndpointId:    ephemeralId,
			AlertEndpoint: endp,
			Details:       req.GetDetails(),
		})
	}

	createImpl := &alertingv1.RoutingNode{
		ConditionId: &corev1.Reference{Id: ephemeralId}, // is used as a unique identifier
		FullAttachedEndpoints: &alertingv1.FullAttachedEndpoints{
			InitialDelay: durationpb.New(time.Duration(time.Second * 0)),
			Items:        fullAttachEndpoint,
			Details:      req.GetDetails(),
		},
	}
	// create condition routing node
	_, err := p.CreateConditionRoutingNode(ctx, createImpl)
	if err != nil {
		return nil, err
	}

	return &alertingv1.EphemeralDispatcherResponse{
		TriggerAlertsRequest: &alertingv1.TriggerAlertsRequest{
			ConditionId: &corev1.Reference{Id: ephemeralId},
			Annotations: map[string]string{},
		},
	}, nil
}
