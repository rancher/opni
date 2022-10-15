package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/alerting/config"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"google.golang.org/protobuf/types/known/emptypb"
)

const endpointPrefix = "/alerting/endpoints"

var locker = sync.Mutex{}

func configFromBackend(p *Plugin, ctx context.Context) (*config.ConfigMapData, error) {
	locker.Lock()
	defer locker.Unlock()
	rawConfig, err := p.opsNode.Fetch(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	config, err := config.NewConfigMapDataFrom(rawConfig.Raw)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func applyConfigToBackend(
	p *Plugin,
	ctx context.Context,
	config *config.ConfigMapData,
	updatedConditionId string,
) error {
	rawData, err := config.Marshal()
	if err != nil {
		return err
	}
	_, err = p.opsNode.Update(ctx, &alertops.AlertingConfig{
		Raw: string(rawData),
	})
	if err != nil {
		return err
	}
	_, err = p.opsNode.Reload(ctx, &alertops.ReloadInfo{
		UpdatedKey: updatedConditionId,
	})
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
	lg.Debugf("dummy id : %s", dummyConditionId)
	impl := req.GetImpl()
	impl.InitialDelay = durationpb.New(time.Duration(time.Second * 0))
	createImpl := &alertingv1alpha.RoutingNode{
		ConditionId:    &corev1.Reference{Id: dummyConditionId}, // is used as a unique identifier
		EndpointId:     &corev1.Reference{Id: ""},               // should never be used
		Implementation: req.GetImpl(),
	}
	recv, err := processEndpointDetails(dummyConditionId, createImpl, req.GetEndpointInfo())
	if err != nil {
		return nil, err
	}
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		lg.Errorf("Failed to fetch plugin options within timeout : %s", err)
		return nil, err
	}
	// interact with pluginBackend
	config, err := configFromBackend(p, ctx)
	if err != nil {
		return nil, err
	}
	lg.Debugf("received config from pluginBackend %v", config)
	config.AppendReceiver(recv)
	config.AppendRoute(recv, createImpl)
	err = applyConfigToBackend(p, ctx, config, dummyConditionId)
	if err != nil {
		return nil, err
	}
	lg.Debug("done reloading")
	// - Trigger it using httpv2 api
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
	// We need to check that endpoint is deleted after a certain amount of time
	// - Delete dummy Endpoint
	go func() {
		time.Sleep(time.Second * 30)
		// this cannot be done in the same context as the above call, otherwise the deadline will be exceeded
		if _, err := p.DeleteConditionRoutingNode(context.Background(), &corev1.Reference{Id: dummyConditionId}); err != nil {
			lg.Errorf("delete test implementation failed with %s", err.Error())
		}
	}()
	return &alertingv1alpha.TestAlertEndpointResponse{}, nil
}

func processEndpointDetails(
	conditionId string,
	req *alertingv1alpha.RoutingNode,
	endpointDetails *alertingv1alpha.AlertEndpoint,
) (*config.Receiver, error) {
	if s := endpointDetails.GetSlack(); s != nil {
		recv, err := config.NewSlackReceiver(conditionId, s)
		if err != nil {
			return nil, err // some validation error
		}
		recv, err = config.WithSlackImplementation(recv, req.Implementation)
		if err != nil {
			return nil, err
		}
		return recv, nil
	}
	if e := endpointDetails.GetEmail(); e != nil {
		recv, err := config.NewEmailReceiver(conditionId, e)
		if err != nil {
			return nil, err // some validation error
		}
		recv, err = config.WithEmailImplementation(recv, req.Implementation)
		if err != nil {
			return nil, err
		}
		return recv, nil
	}
	return nil, validation.Error("Unhandled endpoint/implementation details")
}

// Called from CreateAlertCondition
func (p *Plugin) CreateConditionRoutingNode(ctx context.Context, req *alertingv1alpha.RoutingNode) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	config, err := configFromBackend(p, ctx)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storageNode.GetEndpointStorage(ctx, req.EndpointId.Id)
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
	err = applyConfigToBackend(p, ctx, config, conditionId)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from UpdateAlertCondition
func (p *Plugin) UpdateConditionRoutingNode(ctx context.Context, req *alertingv1alpha.RoutingNode) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	config, err := configFromBackend(p, ctx)
	if err != nil {
		return nil, err
	}
	// get the base which is a notification endpoint
	endpointDetails, err := p.storageNode.GetEndpointStorage(ctx, req.EndpointId.Id)
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
	err = applyConfigToBackend(p, ctx, config, req.ConditionId.Id)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Called from DeleteAlertCondition
// Id must be a conditionId
func (p *Plugin) DeleteConditionRoutingNode(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	config, err := configFromBackend(p, ctx)
	if err != nil {
		return nil, err
	}
	if err := config.DeleteReceiver(req.Id); err != nil {
		return nil, err
	}
	if err := config.DeleteRoute(req.Id); err != nil {
		return nil, err
	}
	err = applyConfigToBackend(p, ctx, config, req.Id)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListRoutingRelationships(ctx context.Context, _ *emptypb.Empty) (*alertingv1alpha.RoutingRelationships, error) {
	// TODO : implement this !!!!
	return nil, nil
}
