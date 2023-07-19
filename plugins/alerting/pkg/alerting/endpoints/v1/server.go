package endpoints

import (
	"context"
	"time"

	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/alerting/storage/opts"
	"github.com/rancher/opni/pkg/alerting/storage/spec"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"

	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ alertingv1.AlertEndpointsServer = (*EndpointServerComponent)(nil)

func (e *EndpointServerComponent) CreateAlertEndpoint(ctx context.Context, req *alertingv1.AlertEndpoint) (*corev1.Reference, error) {
	if !e.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	newId := shared.NewAlertingRefId()
	req.Id = newId
	req.LastUpdated = timestamppb.Now()
	if err := e.endpointStorage.Get().Put(ctx, newId, req); err != nil {
		return nil, err
	}
	return &corev1.Reference{
		Id: newId,
	}, nil
}

func (e *EndpointServerComponent) GetAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertEndpoint, error) {
	if !e.Initialized() {
		return nil, status.Error(codes.Unavailable, "Endpoint server is not yet available")
	}
	endp, err := e.endpointStorage.Get().Get(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	return endp, nil
}

func (e *EndpointServerComponent) UpdateAlertEndpoint(ctx context.Context, req *alertingv1.UpdateAlertEndpointRequest) (*alertingv1.ConditionReferenceList, error) {
	if !e.Initialized() {
		return nil, status.Error(codes.Unavailable, "Endpoint server is not yet available")
	}
	if err := unredactSecrets(ctx, e.endpointStorage.Get(), req.Id.Id, req.GetUpdateAlert()); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	resp, err := e.notifications.ListRoutingRelationships(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	dependentConditions := resp.RoutingRelationships[req.Id.Id]
	if dependentConditions != nil && !req.ForceUpdate {
		return dependentConditions, nil
	}
	// force the new endpoint to preserve the original endpoint id
	req.UpdateAlert.Id = req.Id.Id
	req.UpdateAlert.LastUpdated = timestamppb.Now()
	if err := e.endpointStorage.Get().Put(ctx, req.Id.Id, req.GetUpdateAlert()); err != nil {
		return nil, err
	}
	if dependentConditions == nil {
		dependentConditions = &alertingv1.ConditionReferenceList{}
	}
	return dependentConditions, nil
}

func (e *EndpointServerComponent) DeleteAlertEndpoint(ctx context.Context, req *alertingv1.DeleteAlertEndpointRequest) (*alertingv1.ConditionReferenceList, error) {
	if !e.Initialized() {
		return nil, status.Error(codes.Unavailable, "Endpoint server is not yet available")
	}
	existing, err := e.GetAlertEndpoint(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return &alertingv1.ConditionReferenceList{}, nil
	}

	resp, err := e.notifications.ListRoutingRelationships(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	dependentConditions := resp.RoutingRelationships[req.Id.Id]
	if dependentConditions != nil && !req.ForceDelete {
		return dependentConditions, nil
	}

	if err := e.endpointStorage.Get().Delete(ctx, req.Id.Id); err != nil {
		return nil, err
	}

	if dependentConditions == nil {
		return &alertingv1.ConditionReferenceList{}, nil
	}

	lop.ForEach(dependentConditions.Items, func(condRef *alertingv1.ConditionReference, _ int) {
		// delete endpoint metadata from each condition
		cond, err := e.conditionStorage.Get().Group(condRef.GroupId).Get(ctx, condRef.Id)
		if err != nil {
			return
		}
		if cond.AttachedEndpoints != nil && len(cond.AttachedEndpoints.Items) > 0 {
			cond.AttachedEndpoints.Items = lo.Filter(cond.AttachedEndpoints.Items, func(item *alertingv1.AttachedEndpoint, _ int) bool {
				return item.EndpointId != req.Id.Id
			})
		}
		if err := e.conditionStorage.Get().Group(condRef.GroupId).Put(ctx, condRef.Id, cond); err != nil {
			return
		}
	})
	return dependentConditions, nil
}
func (e *EndpointServerComponent) ToggleNotifications(ctx context.Context, req *alertingv1.ToggleRequest) (*emptypb.Empty, error) {
	if !e.Initialized() {
		return nil, status.Error(codes.Unavailable, "Endpoint server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	endp, err := e.endpointStorage.Get().Get(ctx, req.Id.Id, opts.WithUnredacted())
	if err != nil {
		return nil, err
	}
	if endp.GetProperties() == nil {
		endp.Properties = map[string]string{}
	}
	if _, ok := endp.Properties[alertingv1.EndpointTagNotifications]; !ok {
		endp.Properties[alertingv1.EndpointTagNotifications] = "true"
	} else {
		delete(endp.Properties, alertingv1.EndpointTagNotifications)
	}
	endp.LastUpdated = timestamppb.Now()
	return &emptypb.Empty{}, e.endpointStorage.Get().Put(ctx, req.Id.Id, endp)
}

func (e *EndpointServerComponent) ListAlertEndpoints(
	ctx context.Context,
	req *alertingv1.ListAlertEndpointsRequest,
) (*alertingv1.AlertEndpointList, error) {
	if !e.Initialized() {
		return nil, status.Error(codes.Unavailable, "Endpoint server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	endpoints, err := e.endpointStorage.Get().List(ctx)
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

	slices.SortFunc(
		items, func(i, j *alertingv1.AlertEndpointWithId) bool {
			return i.Endpoint.Name < j.Endpoint.Name
		},
	)
	return &alertingv1.AlertEndpointList{Items: items}, nil
}

func (e *EndpointServerComponent) TestAlertEndpoint(ctx context.Context, req *alertingv1.TestAlertEndpointRequest) (*alertingv1.TestAlertEndpointResponse, error) {
	if !e.Initialized() {
		return nil, status.Error(codes.Unavailable, "Endpoint server is not yet available")
	}
	if req.Endpoint == nil {
		return nil, validation.Error("Endpoint must be set")
	}
	// if it has an Id it needs to be unredacted
	if req.Endpoint.Id != "" {
		unredactSecrets(ctx, e.endpointStorage.Get(), req.Endpoint.Id, req.Endpoint)
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	details := &alertingv1.EndpointImplementation{
		Title: "Test Alert Endpoint",
		Body:  "Opni Alerting is sending you a test alert to verify your alert endpoint configuration.",
	}

	ephemeralId := shared.NewAlertingRefId()
	createImpl := &alertingv1.FullAttachedEndpoints{
		InitialDelay: durationpb.New(time.Duration(time.Second * 0)),
		Items: []*alertingv1.FullAttachedEndpoint{
			{
				EndpointId:    ephemeralId,
				AlertEndpoint: req.GetEndpoint(),
				Details:       details,
			},
		},
		ThrottlingDuration: durationpb.New(time.Duration(time.Second * 1)),
		Details:            details,
	}

	// - create ephemeral dispatcher
	router, err := e.routerStorage.Get().Get(ctx, shared.SingleConfigId)
	if err != nil {
		return nil, err
	}

	ns := "test"
	if err := router.SetNamespaceSpec("test", ephemeralId, createImpl); err != nil {
		return nil, err
	}
	go func() { // create, trigger, delete
		ctx := e.ctx
		e.ManualSync(ctx, []string{shared.SingleConfigId}, e.routerStorage.Get())

		_, err := e.notifications.TriggerAlerts(ctx, &alertingv1.TriggerAlertsRequest{
			ConditionId: &corev1.Reference{Id: ephemeralId},
			Namespace:   ns,
			Annotations: map[string]string{
				message.NotificationContentHeader:  "Test notification",
				message.NotificationContentSummary: "Admin has sent a test notification",
			},
			Labels: map[string]string{
				ns: ephemeralId,
			},
		})
		if err != nil {
			e.logger.Errorf("Failed to trigger alert %s", err)
		}

		// - delete ephemeral dispatcher
		if err := router.SetNamespaceSpec("test", ephemeralId, &alertingv1.FullAttachedEndpoints{
			Items: []*alertingv1.FullAttachedEndpoint{},
		}); err != nil {
			return
		}
	}()

	return &alertingv1.TestAlertEndpointResponse{}, nil
}

func unredactSecrets(
	ctx context.Context,
	store spec.EndpointStorage,
	endpointId string,
	endp *alertingv1.AlertEndpoint,
) error {
	unredacted, err := store.Get(ctx, endpointId, opts.WithUnredacted())
	if err != nil {
		return err
	}
	endp.UnredactSecrets(unredacted)
	return nil
}
