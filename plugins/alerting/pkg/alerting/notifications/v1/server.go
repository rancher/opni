package notifications

import (
	"context"
	"fmt"
	"math"
	"time"

	"slices"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/client"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
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

func (n *NotificationServerComponent) TestAlertEndpoint(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	ctxca, ca := context.WithTimeout(ctx, 1*time.Second)
	defer ca()

	endp, err := n.endpointStorage.GetContext(ctxca)
	if err != nil {
		return nil, status.Error(codes.Unavailable, fmt.Sprintf("failed to get endpoit storage : %s", err.Error()))
	}
	if _, err := endp.Get(ctx, ref.Id); err != nil {
		return nil, fmt.Errorf("testing an endpoint requires it is loaded : %w", err)
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	err = n.Client.AlertClient().PostAlarm(
		ctx,
		client.AlertObject{
			Id: ref.Id,
			Labels: map[string]string{
				message.NotificationPropertyOpniUuid: ref.Id,
				message.TestNamespace:                ref.Id,
			},
			Annotations: map[string]string{
				message.NotificationContentHeader:  "Test notification",
				message.NotificationContentSummary: "Admin has sent a test notification",
			},
		},
	)

	return &emptypb.Empty{}, err
}

func (n *NotificationServerComponent) TriggerAlerts(ctx context.Context, req *alertingv1.TriggerAlertsRequest) (*alertingv1.TriggerAlertsResponse, error) {
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

	if _, ok := req.Labels[message.NotificationPropertyOpniUuid]; !ok {
		req.Labels[message.NotificationPropertyOpniUuid] = req.ConditionId.Id
	}

	if _, ok := req.Annotations[shared.BackendConditionNameLabel]; !ok {
		req.Annotations[shared.BackendConditionNameLabel] = req.ConditionName
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	err := n.Client.AlertClient().PostAlarm(
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

	if _, ok := req.Labels[message.NotificationPropertyOpniUuid]; !ok {
		req.Labels[message.NotificationPropertyOpniUuid] = req.ConditionId.Id
	}

	if _, ok := req.Annotations[shared.BackendConditionNameLabel]; !ok {
		req.Annotations[shared.BackendConditionNameLabel] = req.ConditionName
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	err := n.Client.AlertClient().ResolveAlert(ctx, client.AlertObject{
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

	if _, ok := req.GetProperties()[message.NotificationPropertyDedupeKey]; !ok {
		req.Properties[message.NotificationPropertyDedupeKey] = util.HashStrings([]string{req.Title, req.Body})
	}

	routingLabels := req.GetRoutingLabels()
	n.mu.Lock()
	defer n.mu.Unlock()
	err := n.Client.AlertClient().PostNotification(
		ctx,
		client.AlertObject{
			Id:          lo.ValueOr(routingLabels, message.NotificationPropertyOpniUuid, uuid.New().String()),
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

	resp, err := n.Client.QueryClient().ListNotificationMessages(ctx, req)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(resp.Items, func(a, b *alertingv1.MessageInstance) int {
		return a.ReceivedAt.AsTime().Compare(b.ReceivedAt.AsTime())
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

	_, err := n.conditionStorage.Get().Group(req.ConditionId.GroupId).Get(ctx, req.ConditionId.Id)
	if err != nil {
		return nil, err
	}
	consistencyInterval := durationpb.New(time.Second * 30)
	req.End = timestamppb.New(req.End.AsTime().Add(consistencyInterval.AsDuration()))

	resp, err := n.Client.QueryClient().ListAlarmMessages(ctx, req)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(resp.Items, func(a, b *alertingv1.MessageInstance) int {
		return a.ReceivedAt.AsTime().Compare(b.ReceivedAt.AsTime())
	})
	return resp, nil
}

func (n *NotificationServerComponent) ListRoutingRelationships(ctx context.Context, _ *emptypb.Empty) (*alertingv1.ListRoutingRelationshipsResponse, error) {
	if !n.Initialized() {
		return nil, status.Error(codes.Unavailable, "Notification server is not yet available")
	}
	groupsIds, err := n.conditionStorage.Get().ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	conds := []*alertingv1.AlertCondition{}
	for _, groupId := range groupsIds {
		groupConds, err := n.conditionStorage.Get().Group(groupId).List(ctx)
		if err != nil {
			return nil, err
		}
		conds = append(conds, groupConds...)
	}
	relationships := map[string]*alertingv1.ConditionReferenceList{}
	for _, c := range conds {
		if c.AttachedEndpoints == nil {
			continue
		}
		for _, ep := range c.AttachedEndpoints.Items {
			if _, ok := relationships[ep.EndpointId]; !ok {
				relationships[ep.EndpointId] = &alertingv1.ConditionReferenceList{
					Items: []*alertingv1.ConditionReference{},
				}
			}
			relationships[ep.EndpointId].Items = append(relationships[ep.EndpointId].Items, &alertingv1.ConditionReference{
				Id:      c.GetId(),
				GroupId: c.GetGroupId(),
			})
		}
	}
	return &alertingv1.ListRoutingRelationshipsResponse{
		RoutingRelationships: relationships,
	}, nil
}
