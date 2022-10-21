package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/tidwall/gjson"

	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1alpha.AlertCondition) (*corev1.Reference, error) {
	lg := p.Logger.With("Handler", "CreateAlertCondition")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := alertingv1alpha.DetailsHasImplementation(req.GetAlertType()); err != nil {
		return nil, shared.WithNotFoundError(fmt.Sprintf("%s", err))
	}
	newId := uuid.New().String()
	_, err := setupCondition(p, lg, ctx, req, newId)
	if err != nil {
		return nil, err
	}

	if !(req.AttachedEndpoints == nil || len(req.AttachedEndpoints.Items) == 0) {
		// FIXME: temporary solution
		endpointItems, err := p.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
		if err != nil {
			return nil, err
		}
		routingNode := &alertingv1alpha.RoutingNode{
			ConditionId: &corev1.Reference{Id: newId},
			FullAttachedEndpoints: &alertingv1alpha.FullAttachedEndpoints{
				Items:              []*alertingv1alpha.FullAttachedEndpoint{},
				InitialDelay:       req.AttachedEndpoints.InitialDelay,
				RepeatInterval:     req.AttachedEndpoints.RepeatInterval,
				ThrottlingDuration: req.AttachedEndpoints.ThrottlingDuration,
				Details:            req.AttachedEndpoints.Details,
			},
		}
		for _, endpointItem := range endpointItems.Items {
			for _, expectedEndpoint := range req.AttachedEndpoints.Items {
				if endpointItem.Id.Id == expectedEndpoint.EndpointId {
					routingNode.FullAttachedEndpoints.Items = append(
						routingNode.FullAttachedEndpoints.Items,
						&alertingv1alpha.FullAttachedEndpoint{
							EndpointId:    endpointItem.Id.Id,
							AlertEndpoint: endpointItem.Endpoint,
							Details:       req.AttachedEndpoints.Details,
						})
				}
			}
		}
		_, err = p.CreateConditionRoutingNode(ctx, routingNode)
		if err != nil {
			return nil, err
		}
	}
	if err := p.storageNode.CreateConditionStorage(ctx, newId, req); err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: newId}, nil
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertCondition, error) {
	return p.storageNode.GetConditionStorage(ctx, ref.Id)
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1alpha.ListAlertConditionRequest) (*alertingv1alpha.AlertConditionList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	keys, items, err := p.storageNode.ListWithKeyConditionStorage(ctx)
	if err != nil {
		return nil, err
	}

	res := &alertingv1alpha.AlertConditionList{}
	for i := range keys {
		res.Items = append(res.Items, &alertingv1alpha.AlertConditionWithId{
			Id:             &corev1.Reference{Id: keys[i]},
			AlertCondition: items[i],
		})
	}
	return res, nil
}

// req.Id is the condition id reference
func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *alertingv1alpha.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	lg := p.Logger.With("handler", "UpdateAlertCondition")
	lg.Debugf("Updating alert condition %s", req.Id)
	overrideLabels := req.UpdateAlert.Labels
	conditionId := req.Id.Id
	existing, err := p.storageNode.GetConditionStorage(ctx, req.Id.Id)
	if err != nil {
		return nil, err
	}

	_, err = setupCondition(p, lg, ctx, req.UpdateAlert, req.Id.Id)
	if err != nil {
		return nil, err
	}
	if !(req.UpdateAlert.AttachedEndpoints == nil || len(req.UpdateAlert.AttachedEndpoints.Items) == 0) {
		endpointItems, err := p.ListAlertEndpoints(ctx, &alertingv1alpha.ListAlertEndpointsRequest{})
		if err != nil {
			return nil, err
		}
		routingNode := &alertingv1alpha.RoutingNode{
			ConditionId: &corev1.Reference{Id: req.Id.Id},
			FullAttachedEndpoints: &alertingv1alpha.FullAttachedEndpoints{
				Items:              []*alertingv1alpha.FullAttachedEndpoint{},
				InitialDelay:       req.UpdateAlert.AttachedEndpoints.InitialDelay,
				RepeatInterval:     req.UpdateAlert.AttachedEndpoints.RepeatInterval,
				ThrottlingDuration: req.UpdateAlert.AttachedEndpoints.ThrottlingDuration,
				Details:            req.UpdateAlert.AttachedEndpoints.Details,
			},
		}
		for _, endpointItem := range endpointItems.Items {
			for _, expectedEndpoint := range req.UpdateAlert.AttachedEndpoints.Items {
				if endpointItem.Id.Id == expectedEndpoint.EndpointId {
					routingNode.FullAttachedEndpoints.Items = append(
						routingNode.FullAttachedEndpoints.Items,
						&alertingv1alpha.FullAttachedEndpoint{
							EndpointId:    endpointItem.Id.Id,
							AlertEndpoint: endpointItem.Endpoint,
							Details:       req.UpdateAlert.AttachedEndpoints.Details,
						})
				}
			}
		}
		if existing.AttachedEndpoints != nil && len(existing.AttachedEndpoints.Items) > 0 {
			// existing condition has active endpoints, so we need to update the routing node
			_, err = p.UpdateConditionRoutingNode(ctx, routingNode)
			if err != nil {
				return nil, err
			}
		} else {
			// existing condition did not have active endpoints so create the routing node
			_, err = p.CreateConditionRoutingNode(ctx, routingNode)
			if err != nil {
				return nil, err
			}
		}

	} else if existing.AttachedEndpoints != nil && len(existing.AttachedEndpoints.Items) > 0 {
		// new condition has new active endpoints, but old one did
		_, err = p.DeleteConditionRoutingNode(ctx, &corev1.Reference{Id: conditionId})
	}
	proto.Merge(existing, req.UpdateAlert)
	existing.Labels = overrideLabels
	if err := p.storageNode.UpdateConditionStorage(ctx, conditionId, existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	lg := p.Logger.With("Handler", "DeleteAlertCondition")
	lg.Debugf("Deleting alert condition %s", ref.Id)
	existing, err := p.storageNode.GetConditionStorage(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	if err := deleteCondition(p, lg, ctx, existing, ref.Id); err != nil {
		return nil, err
	}
	lg.Debugf("Deleted condition %s must clean up its existing endpoint implementation", ref.Id)
	if !(existing.AttachedEndpoints == nil || len(existing.AttachedEndpoints.Items) == 0) {
		_, err = p.DeleteConditionRoutingNode(ctx, ref)
		if err != nil {
			return nil, err
		}
	}
	err = p.storageNode.DeleteConditionStorage(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) AlertConditionStatus(ctx context.Context, ref *corev1.Reference) (*alertingv1alpha.AlertStatusResponse, error) {
	lg := p.Logger.With("handler", "AlertConditionStatus")
	lg.Debugf("Getting alert condition status %s", ref.Id)

	_, err := p.storageNode.GetConditionStorage(ctx, ref.Id)
	if err != nil {
		lg.Errorf("failed to find condition with id %s in storage : %s", ref.Id, err)
		return nil, shared.WithNotFoundErrorf("%s", err)
	}

	defaultState := &alertingv1alpha.AlertStatusResponse{
		State: alertingv1alpha.AlertConditionState_OK,
	}
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		lg.Errorf("Failed to get alerting options : %s", err)
		return nil, err
	}
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, options)
	if err != nil {
		return nil, err
	}
	var result gjson.Result
	apiNode := backend.NewAlertManagerGetAlertsClient(
		availableEndpoint,
		ctx,
		backend.WithLogger(lg),
		backend.WithExpectClosure(func(resp *http.Response) error {

			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				lg.Errorf("failed to read body : %s", err)
				return err
			}
			result = gjson.Get(string(b), "")
			return nil
		}),
	)
	err = apiNode.DoRequest()
	if err != nil {
		return nil, err
	}
	if !result.Exists() {
		return defaultState, nil
	}
	for _, alert := range result.Array() {
		receiverName := gjson.Get(alert.String(), "receiver.name")
		if receiverName.String() == ref.Id {
			state := gjson.Get(alert.String(), "status.state")
			switch state.String() {
			case models.AlertStatusStateActive:
				return &alertingv1alpha.AlertStatusResponse{
					State: alertingv1alpha.AlertConditionState_FIRING,
				}, nil
			case models.AlertStatusStateSuppressed:
				return &alertingv1alpha.AlertStatusResponse{
					State: alertingv1alpha.AlertConditionState_SILENCED,
				}, nil

			case models.AlertStatusStateUnprocessed:
				return defaultState, nil
			default:
				return defaultState, nil
			}
		}
	}
	return defaultState, nil
}

func (p *Plugin) ActivateSilence(ctx context.Context, req *alertingv1alpha.SilenceRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := p.storageNode.GetConditionStorage(ctx, req.ConditionId.Id)
	if err != nil {
		return nil, err
	}
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, options)
	if err != nil {
		return nil, err
	}
	var silenceID *string
	if existing.Silence != nil {
		silenceID = &existing.Silence.SilenceId
	}
	respSilence := &backend.PostSilencesResponse{}
	apiNode := backend.NewAlertManagerPostSilenceClient(
		availableEndpoint,
		ctx,
		backend.WithLogger(p.Logger),
		backend.WithPostSilenceBody(req.ConditionId.Id, req.Duration.AsDuration(), silenceID),
		backend.WithExpectClosure(func(resp *http.Response) error {
			if resp.StatusCode != 200 {
				return fmt.Errorf("failed to create silence : %s", resp.Status)
			}
			return json.NewDecoder(resp.Body).Decode(respSilence)
		}),
	)
	err = apiNode.DoRequest()
	if err != nil {
		p.Logger.Errorf("failed to post silence : %s", err)
		return nil, err
	}
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = &alertingv1alpha.SilenceInfo{ // not exact, butno one will notice
		SilenceId: respSilence.GetSilenceId(),
		StartsAt:  timestamppb.Now(),
		EndsAt:    timestamppb.New(time.Now().Add(req.Duration.AsDuration())),
	}
	// update K,V with new silence info for the respective condition
	proto.Merge(existing, newCondition)
	if err := p.storageNode.UpdateConditionStorage(ctx, req.ConditionId.Id, existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// DeactivateSilence req.Id is a condition id reference
func (p *Plugin) DeactivateSilence(ctx context.Context, req *corev1.Reference) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		return nil, err
	}
	existing, err := p.storageNode.GetConditionStorage(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if existing.Silence == nil {
		return nil, validation.Errorf("could not find existing silence for condition %s", req.Id)
	}
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, options)
	if err != nil {
		return nil, err
	}
	apiNode := backend.NewAlertManagerDeleteSilenceClient(
		availableEndpoint,
		existing.Silence.SilenceId,
		ctx)
	err = apiNode.DoRequest()
	if err != nil {
		p.Logger.Errorf("failed to delete silence : %s", err)
		return nil, err
	}

	// update existing proto with the silence info
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = nil
	// update K,V with new silence info for the respective condition
	proto.Merge(existing, newCondition)
	if err := p.storageNode.UpdateConditionStorage(ctx, req.Id, existing); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListAlertConditionChoices(ctx context.Context, req *alertingv1alpha.AlertDetailChoicesRequest) (*alertingv1alpha.ListAlertTypeDetails, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := alertingv1alpha.EnumHasImplementation(req.GetAlertType()); err != nil {
		return nil, err
	}
	return handleChoicesByType(p, ctx, req)
}

func (p *Plugin) Timeline(ctx context.Context, req *alertingv1alpha.TimelineRequest) (*alertingv1alpha.TimelineResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ids, conditions, err := p.storageNode.ListWithKeyConditionStorage(ctx)
	if err != nil {
		return nil, err
	}
	resp := &alertingv1alpha.TimelineResponse{
		Items: make(map[string]*alertingv1alpha.ActiveWindows),
	}
	for idx := range conditions {
		resp.Items[ids[idx]] = &alertingv1alpha.ActiveWindows{
			Windows: make([]*alertingv1alpha.ActiveWindow, 0),
		}
		numWindows := rand.Intn(10) + 1
		for i := 0; i < numWindows; i++ {
			startTime := time.Now()
			startTime.Add(-time.Duration(rand.Int31n(24)+1) * time.Hour)
			endTime := time.Now()
			endTime.Add(-time.Duration(rand.Int31n(24)+1) * time.Hour)
			if endTime.UnixNano() < startTime.UnixNano() {
				temp := startTime
				startTime = endTime
				endTime = temp
			}
			typeRandom := rand.Intn(4)
			var t alertingv1alpha.TimelineType
			if typeRandom == 0 {
				t = alertingv1alpha.TimelineType_Timeline_Silenced
			} else {
				t = alertingv1alpha.TimelineType_Timeline_Alerting
			}
			resp.Items[ids[idx]].Windows = append(resp.Items[ids[idx]].Windows, &alertingv1alpha.ActiveWindow{
				Start: timestamppb.New(startTime),
				End:   timestamppb.New(endTime),
				Type:  t,
			})
		}
	}
	return resp, nil
}
