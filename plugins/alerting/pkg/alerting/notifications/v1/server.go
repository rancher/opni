package notifications

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// groups notifications by the duration "context" that is sending them
// resolveContext's value will depend on the values the notification routing tree holds
var resolveContext time.Duration

func init() {
	notificationSubtree := routing.NotificationSubTreeValues()
	maxDur := time.Duration(0)
	for _, node := range notificationSubtree {
		dur := node.B.InitialDelay + node.B.ThrottlingDuration
		if dur > maxDur {
			maxDur = dur
		}
	}
	resolveContext = time.Microsecond * time.Duration((math.Round(float64(maxDur.Microseconds()) * 1.2)))
}

var _ (alertingv1.AlertNotificationsServer) = (*NotificationServerComponent)(nil)

func (n *NotificationServerComponent) TriggerAlerts(ctx context.Context, req *alertingv1.TriggerAlertsRequest) (*alertingv1.TriggerAlertsResponse, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	lg := n.logger.With("Handler", "TriggerAlerts")
	lg.Debugf("Received request to trigger alerts  on condition %s", req.GetConditionId())
	lg.Debugf("Received alert annotations : %s", req.Annotations)

	// This logic is intended to
	// 1) Provide a safeguard to ensure that external callers of the API will not cause nil pointer map dereferences in the AlertManager adapter logic
	// 2) Set the required information if it is not already embedded in the postable Alert's information.
	// 3) Ensure that callers of the API are efficiently & uniquely routed when posted to our construction
	// of the AlertManager routing tree, regardless if there are specific routes & receivers to handle this set of information
	if _, ok := req.Labels[req.Namespace]; !ok {
		req.Labels[req.Namespace] = req.ConditionId.Id
	}

	if _, ok := req.Labels[alertingv1.NotificationPropertyOpniUuid]; !ok {
		req.Labels[alertingv1.NotificationPropertyOpniUuid] = req.ConditionId.Id
	}

	if _, ok := req.Annotations[shared.BackendConditionNameLabel]; !ok {
		req.Annotations[shared.BackendConditionNameLabel] = req.ConditionName
	}

	err := n.Client.PostAlarm(
		ctx,
		client.AlertObject{
			Id:          req.ConditionId.Id,
			Labels:      req.Labels,
			Annotations: req.Annotations,
		},
	)

	return &alertingv1.TriggerAlertsResponse{}, err
}

func (n *NotificationServerComponent) ResolveAlerts(ctx context.Context, req *alertingv1.ResolveAlertsRequest) (*alertingv1.ResolveAlertsResponse, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// This logic is intended to
	// 1) Provide a safeguard to ensure that external callers of the API will not cause nil pointer map dereferences in the AlertManager adapter logic
	// 2) Set the required information if it is not already embedded in the postable Alert's information.
	// 3) Ensure that callers of the API are efficiently & uniquely routed when posted to our construction
	// of the AlertManager routing tree, regardless if there are specific routes & receivers to handle this set of information
	if _, ok := req.Labels[req.Namespace]; !ok {
		req.Labels[req.Namespace] = req.ConditionId.Id
	}

	if _, ok := req.Labels[alertingv1.NotificationPropertyOpniUuid]; !ok {
		req.Labels[alertingv1.NotificationPropertyOpniUuid] = req.ConditionId.Id
	}

	if _, ok := req.Annotations[shared.BackendConditionNameLabel]; !ok {
		req.Annotations[shared.BackendConditionNameLabel] = req.ConditionName
	}

	err := n.Client.ResolveAlert(ctx, client.AlertObject{
		Id:          req.ConditionId.Id,
		Labels:      req.Labels,
		Annotations: req.Annotations,
	})

	return &alertingv1.ResolveAlertsResponse{}, err
}

func (n *NotificationServerComponent) PushNotification(ctx context.Context, req *alertingv1.Notification) (*emptypb.Empty, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	req.Sanitize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	if _, ok := req.GetProperties()[alertingv1.NotificationPropertyDedupeKey]; !ok {
		req.Properties[alertingv1.NotificationPropertyDedupeKey] = util.HashStrings([]string{req.Title, req.Body})
	}

	routingLabels := req.GetRoutingLabels()
	err := n.Client.PostNotification(
		ctx,
		client.AlertObject{
			Id:          lo.ValueOr(routingLabels, alertingv1.NotificationPropertyOpniUuid, uuid.New().String()),
			Labels:      routingLabels,
			Annotations: req.GetRoutingAnnotations(),
		},
	)
	return &emptypb.Empty{}, err
}

func (n *NotificationServerComponent) ListNotifications(ctx context.Context, req *alertingv1.ListNotificationRequest) (*alertingv1.ListMessageResponse, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	req.Sanitize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	resp, err := n.Client.ListNotificationMessages(ctx, req)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(resp.Items, func(a, b *alertingv1.MessageInstance) bool {
		return a.ReceivedAt.AsTime().Before(b.ReceivedAt.AsTime())
	})
	return resp, nil
}

func (n *NotificationServerComponent) ListAlarmMessages(ctx context.Context, req *alertingv1.ListAlarmMessageRequest) (*alertingv1.ListMessageResponse, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	req.Sanitize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	cond, err := n.conditionStorage.Get().Get(ctx, req.ConditionId)
	if err != nil {
		return nil, err
	}
	consistencyInterval := durationpb.New(time.Second * 30)
	if cond.AttachedEndpoints != nil {
		consistencyInterval = cond.AttachedEndpoints.InitialDelay
	}
	req.End = timestamppb.New(req.End.AsTime().Add(consistencyInterval.AsDuration()))

	resp, err := n.Client.ListAlarmMessages(ctx, req)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(resp.Items, func(a, b *alertingv1.MessageInstance) bool {
		return a.ReceivedAt.AsTime().Before(b.ReceivedAt.AsTime())
	})
	return resp, nil
}

func (n *NotificationServerComponent) ListRoutingRelationships(ctx context.Context, _ *emptypb.Empty) (*alertingv1.ListRoutingRelationshipsResponse, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	conds, err := n.conditionStorage.Get().List(ctx)
	if err != nil {
		return nil, err
	}
	relationships := map[string]*corev1.ReferenceList{}
	for _, c := range conds {
		if c.AttachedEndpoints != nil && len(c.AttachedEndpoints.Items) > 0 {
			refs := &corev1.ReferenceList{
				Items: lo.Map(
					c.AttachedEndpoints.Items,
					func(endp *alertingv1.AttachedEndpoint, _ int) *corev1.Reference {
						return &corev1.Reference{
							Id: endp.EndpointId,
						}
					}),
			}
			relationships[c.Id] = refs
		}
	}
	return &alertingv1.ListRoutingRelationshipsResponse{
		RoutingRelationships: relationships,
	}, nil
}
