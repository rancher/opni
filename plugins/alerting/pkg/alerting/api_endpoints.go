package alerting

// import (
// 	"context"
// 	"time"

// 	"github.com/rancher/opni/pkg/alerting/shared"
// 	"github.com/rancher/opni/pkg/alerting/storage"
// 	"github.com/rancher/opni/pkg/alerting/storage/opts"
// 	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
// 	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
// 	"github.com/rancher/opni/pkg/validation"
// 	"github.com/samber/lo"
// 	lop "github.com/samber/lo/parallel"
// 	"golang.org/x/exp/slices"
// 	"google.golang.org/protobuf/types/known/durationpb"
// 	"google.golang.org/protobuf/types/known/emptypb"
// 	"google.golang.org/protobuf/types/known/timestamppb"
// )

// func unredactSecrets(
// 	ctx context.Context,
// 	store storage.AlertingClientSet,
// 	endpointId string,
// 	endp *alertingv1.AlertEndpoint,
// ) error {
// 	unredacted, err := store.Endpoints().Get(ctx, endpointId, opts.WithUnredacted())
// 	if err != nil {
// 		return err
// 	}
// 	endp.UnredactSecrets(unredacted)
// 	return nil
// }

// func (p *Plugin) CreateAlertEndpoint(ctx context.Context, req *alertingv1.AlertEndpoint) (*corev1.Reference, error) {
// 	if err := req.Validate(); err != nil {
// 		return nil, err
// 	}
// 	newId := shared.NewAlertingRefId()
// 	req.Id = newId
// 	req.LastUpdated = timestamppb.Now()
// 	if err := p.storageClientSet.Get().Endpoints().Put(ctx, newId, req); err != nil {
// 		return nil, err
// 	}
// 	return &corev1.Reference{
// 		Id: newId,
// 	}, nil
// }

// func (p *Plugin) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertEndpoint, error) {
// 	endp, err := p.storageClientSet.Get().Endpoints().Get(ctx, ref.Id)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return endp, nil
// }

// func (p *Plugin) UpdateAlertEndpoint(ctx context.Context, req *alertingv1.UpdateAlertEndpointRequest) (*alertingv1.InvolvedConditions, error) {
// 	if err := unredactSecrets(ctx, p.storageClientSet.Get(), req.Id.Id, req.GetUpdateAlert()); err != nil {
// 		return nil, err
// 	}
// 	if err := req.Validate(); err != nil {
// 		return nil, err
// 	}
// 	resp, err := p.ListRoutingRelationships(ctx, &emptypb.Empty{})
// 	if err != nil {
// 		return nil, err
// 	}
// 	refList := resp.GetInvolvedConditions(req.Id.Id)
// 	if len(refList.Items) > 0 && !req.ForceUpdate {
// 		return refList, nil
// 	}
// 	// force the new endpoint to preserve the original endpoint id
// 	req.UpdateAlert.Id = req.Id.Id
// 	req.UpdateAlert.LastUpdated = timestamppb.Now()
// 	if err := p.storageClientSet.Get().Endpoints().Put(ctx, req.Id.Id, req.GetUpdateAlert()); err != nil {
// 		return nil, err
// 	}
// 	return refList, nil
// }

// func (p *Plugin) ListAlertEndpoints(ctx context.Context,
// 	req *alertingv1.ListAlertEndpointsRequest) (*alertingv1.AlertEndpointList, error) {
// 	if err := req.Validate(); err != nil {
// 		return nil, err
// 	}
// 	endpoints, err := p.storageClientSet.Get().Endpoints().List(ctx)
// 	if err != nil {
// 		return nil, err
// 	}
// 	items := []*alertingv1.AlertEndpointWithId{}
// 	for _, endp := range endpoints {
// 		items = append(items, &alertingv1.AlertEndpointWithId{
// 			Id:       &corev1.Reference{Id: endp.Id},
// 			Endpoint: endp,
// 		})
// 	}

// 	slices.SortFunc(
// 		items, func(i, j *alertingv1.AlertEndpointWithId) bool {
// 			return i.Endpoint.Name < j.Endpoint.Name
// 		},
// 	)
// 	return &alertingv1.AlertEndpointList{Items: items}, nil
// }

// func (p *Plugin) DeleteAlertEndpoint(ctx context.Context, req *alertingv1.DeleteAlertEndpointRequest) (*alertingv1.InvolvedConditions, error) {
// 	existing, err := p.GetAlertEndpoint(ctx, req.Id)
// 	if err != nil {
// 		return nil, err
// 	}
// 	if existing == nil {
// 		return &alertingv1.InvolvedConditions{}, nil
// 	}

// 	resp, err := p.ListRoutingRelationships(ctx, &emptypb.Empty{})
// 	if err != nil {
// 		return nil, err
// 	}
// 	refList := resp.GetInvolvedConditions(req.Id.Id)
// 	if len(refList.Items) > 0 && !req.ForceDelete {
// 		return refList, nil
// 	}

// 	if err := p.storageClientSet.Get().Endpoints().Delete(ctx, req.Id.Id); err != nil {
// 		return nil, err
// 	}
// 	lop.ForEach(refList.Items, func(condRef *corev1.Reference, _ int) {
// 		// delete endpoint metadata from each condition
// 		cond, err := p.storageClientSet.Get().Conditions().Get(ctx, condRef.Id)
// 		if err != nil {
// 			return
// 		}
// 		if cond.AttachedEndpoints != nil && len(cond.AttachedEndpoints.Items) > 0 {
// 			cond.AttachedEndpoints.Items = lo.Filter(cond.AttachedEndpoints.Items, func(item *alertingv1.AttachedEndpoint, _ int) bool {
// 				return item.EndpointId != req.Id.Id
// 			})
// 		}
// 		if err := p.storageClientSet.Get().Conditions().Put(ctx, condRef.Id, cond); err != nil {
// 			return
// 		}
// 	})

// 	return refList, nil
// }

// func (p *Plugin) TestAlertEndpoint(ctx context.Context, req *alertingv1.TestAlertEndpointRequest) (*alertingv1.TestAlertEndpointResponse, error) {
// 	// lg := p.Logger.With("Handler", "TestAlertEndpoint")
// 	if req.Endpoint == nil {
// 		return nil, validation.Error("Endpoint must be set")
// 	}
// 	// if it has an Id it needs to be unredacted
// 	if req.Endpoint.Id != "" {
// 		unredactSecrets(ctx, p.storageClientSet.Get(), req.Endpoint.Id, req.Endpoint)
// 	}
// 	if err := req.Validate(); err != nil {
// 		return nil, err
// 	}
// 	details := &alertingv1.EndpointImplementation{
// 		Title: "Test Alert Endpoint",
// 		Body:  "Opni Alerting is sending you a test alert to verify your alert endpoint configuration.",
// 	}

// 	ephemeralId := shared.NewAlertingRefId()
// 	createImpl := &alertingv1.FullAttachedEndpoints{
// 		InitialDelay: durationpb.New(time.Duration(time.Second * 0)),
// 		Items: []*alertingv1.FullAttachedEndpoint{
// 			{
// 				EndpointId:    ephemeralId,
// 				AlertEndpoint: req.GetEndpoint(),
// 				Details:       details,
// 			},
// 		},
// 		ThrottlingDuration: durationpb.New(time.Duration(time.Second * 1)),
// 		Details:            details,
// 	}

// 	// - create ephemeral dispatcher
// 	router, err := p.storageClientSet.Get().Routers().Get(ctx, shared.SingleConfigId)
// 	if err != nil {
// 		return nil, err
// 	}

// 	ns := "test"
// 	if err := router.SetNamespaceSpec("test", ephemeralId, createImpl); err != nil {
// 		return nil, err
// 	}

// 	go func() { // create, trigger, delete
// 		ctx := p.Ctx
// 		p.opsNode.SendManualSyncRequest(ctx, []string{shared.SingleConfigId}, p.storageClientSet.Get().Routers())

// 		_, err := p.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
// 			ConditionId: &corev1.Reference{Id: ephemeralId},
// 			Namespace:   ns,
// 			Annotations: map[string]string{
// 				shared.OpniHeaderAnnotations: "Test notification",
// 				shared.OpniBodyAnnotations:   "Admin has sent a test notification",
// 			},
// 			Labels: map[string]string{
// 				ns: ephemeralId,
// 			},
// 		})
// 		if err != nil {
// 			p.Logger.Errorf("Failed to trigger alert %s", err)
// 		}

// 		// - delete ephemeral dispatcher
// 		if err := router.SetNamespaceSpec("test", ephemeralId, &alertingv1.FullAttachedEndpoints{
// 			Items: []*alertingv1.FullAttachedEndpoint{},
// 		}); err != nil {
// 			return
// 		}
// 	}()

// 	return &alertingv1.TestAlertEndpointResponse{}, nil
// }

// func (p *Plugin) ToggleNotifications(ctx context.Context, req *alertingv1.ToggleRequest) (*emptypb.Empty, error) {
// 	if err := req.Validate(); err != nil {
// 		return nil, err
// 	}

// 	endp, err := p.storageClientSet.Get().Endpoints().Get(ctx, req.Id.Id, opts.WithUnredacted())
// 	if err != nil {
// 		return nil, err
// 	}
// 	if endp.GetProperties() == nil {
// 		endp.Properties = map[string]string{}
// 	}
// 	if _, ok := endp.Properties[alertingv1.EndpointTagNotifications]; !ok {
// 		endp.Properties[alertingv1.EndpointTagNotifications] = "true"
// 	} else {
// 		delete(endp.Properties, alertingv1.EndpointTagNotifications)
// 	}
// 	endp.LastUpdated = timestamppb.Now()
// 	return &emptypb.Empty{}, p.storageClientSet.Get().Endpoints().Put(ctx, req.Id.Id, endp)
// }
