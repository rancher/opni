package alerting

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"sync"
	"time"
)

var locker = sync.Mutex{}

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
	if s := req.EndpointInfo.GetSlack(); s != nil {
		typeName = "slack"
	}
	if e := req.EndpointInfo.GetEmail(); e != nil {
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
					AlertEndpoint: req.EndpointInfo,
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
	go func() {
		//TODO: FIXME: retrier backoff
		time.Sleep(time.Second * 30)
		// this cannot be done in the same context as the above call, otherwise the deadline will be exceeded
		if _, err := p.DeleteConditionRoutingNode(context.Background(), &corev1.Reference{Id: dummyConditionId}); err != nil {
			lg.Errorf("delete test implementation failed with %s", err.Error())
		}
	}()
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
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig, req.ConditionId.Id)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) UpdateConditionRoutingNode(ctx context.Context, req *alertingv1alpha.RoutingNode) (*emptypb.Empty, error) {
	// TODO : implement this
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
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig, req.ConditionId.Id)
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
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig, req.Id)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListRoutingRelationships(ctx context.Context, _ *emptypb.Empty) (*alertingv1alpha.RoutingRelationships, error) {
	// TODO : implement this !!!!
	return nil, nil
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
	//TODO: FIXME: This is a hack to get around the fact that we don't have a conditionId here
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig, "")
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteIndividualEndpointInRoutingNode(ctx context.Context, reference *corev1.Reference) (*emptypb.Empty, error) {
	amConfig, internalConfig, err := p.opsNode.ConfigFromBackend(ctx)
	if err != nil {
		return nil, err
	}
	err = amConfig.DeleteIndividualEndpointNode(reference.Id, internalConfig)
	if err != nil {
		return nil, err
	}
	//TODO: FIXME: This is a hack to get around the fact that we don't have a conditionId here
	err = p.opsNode.ApplyConfigToBackend(ctx, amConfig, internalConfig, "")
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
