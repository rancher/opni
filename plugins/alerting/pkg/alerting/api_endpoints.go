package alerting

import (
	"context"
	"fmt"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage"
	"github.com/rancher/opni/pkg/alerting/storage/storage_opts"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func unredactSecrets(
	ctx context.Context,
	store storage.AlertingClientSet,
	endpointId string,
	endp *alertingv1.AlertEndpoint,
) error {
	unredacted, err := store.Endpoints().Get(ctx, endpointId, storage_opts.WithUnredacted())
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
	newId := shared.NewAlertingRefId()
	req.Id = newId
	req.LastUpdated = timestamppb.Now()
	if err := p.storageClientSet.Get().Endpoints().Put(ctx, newId, req); err != nil {
		return nil, err
	}
	return &corev1.Reference{
		Id: newId,
	}, nil
}

func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertEndpoint, error) {
	endp, err := p.storageClientSet.Get().Endpoints().Get(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	return endp, nil
}

func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1.UpdateAlertEndpointRequest) (*alertingv1.InvolvedConditions, error) {
	if err := unredactSecrets(ctx, p.storageClientSet.Get(), req.Id.Id, req.GetUpdateAlert()); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	refList := []*corev1.Reference{}
	//TODO : check involved conditions

	req.UpdateAlert.LastUpdated = timestamppb.Now()
	if err := p.storageClientSet.Get().Endpoints().Put(ctx, req.Id.Id, req.GetUpdateAlert()); err != nil {
		return nil, err
	}
	return &alertingv1.InvolvedConditions{
		Items: refList,
	}, nil
}

func (p *Plugin) ListAlertEndpoints(ctx context.Context,
	req *alertingv1.ListAlertEndpointsRequest) (*alertingv1.AlertEndpointList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	endpoints, err := p.storageClientSet.Get().Endpoints().List(ctx)
	if err != nil {
		return nil, err
	}
	items := []*alertingv1.AlertEndpointWithId{}
	for _, endp := range endpoints {
		items = append(items, &alertingv1.AlertEndpointWithId{
			Id:       &corev1.Reference{Id: endp.Id},
			Endpoint: endp,
		})
	}
	return &alertingv1.AlertEndpointList{Items: items}, nil
}

func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, req *alertingv1.DeleteAlertEndpointRequest) (*alertingv1.InvolvedConditions, error) {
	existing, err := p.GetAlertEndpoint(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return &alertingv1.InvolvedConditions{}, nil
	}

	refList := []*corev1.Reference{}
	//TODO : list involved conditions

	if err := p.storageClientSet.Get().Endpoints().Delete(ctx, req.Id.Id); err != nil {
		return nil, err
	}

	return &alertingv1.InvolvedConditions{
		Items: refList,
	}, nil
}

func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1.TestAlertEndpointRequest) (*alertingv1.TestAlertEndpointResponse, error) {
	// lg := p.Logger.With("Handler", "TestAlertEndpoint")
	if req.Endpoint == nil {
		return nil, validation.Error("Endpoint must be set")
	}
	// if it has an Id it needs to be unredacted
	if req.Endpoint.Id == "" {
		unredactSecrets(ctx, p.storageClientSet.Get(), req.Endpoint.Id, req.Endpoint)
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// TODO : maybe this process will be simplified
	// - create details

	// - create ephemeral dispatcher

	// - wait for alerting syncer sync

	// - trigger alert

	// - delete ephemeral dispatcher

	// // need to trigger multiple alerts here, a reload can cause a context.Cancel()
	// // to any pending post requests, resulting in the alert never being sent
	// apiNode := backend.NewAlertManagerPostAlertClient(
	// 	ctx,
	// 	availableEndpoint,
	// 	backend.WithLogger(lg),
	// 	backend.WithPostAlertBody(triggerReq.TriggerAlertsRequest.ConditionId.GetId(), nil),
	// 	backend.WithDefaultRetrier(),
	// 	backend.WithExpectClosure(backend.NewExpectStatusCodes([]int{200, 422})))
	// err = apiNode.DoRequest()
	// if err != nil {
	// 	lg.Errorf("Failed to post alert to alertmanager : %s", err)
	// }

	return &alertingv1.TestAlertEndpointResponse{}, nil
}

func (p *Plugin) EphemeralDispatcher(ctx context.Context, req *alertingv1.EphemeralDispatcherRequest) (*alertingv1.EphemeralDispatcherResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// TODO : create a dispatcher on the router node, process may change

	return nil, fmt.Errorf("not implemented")
}
