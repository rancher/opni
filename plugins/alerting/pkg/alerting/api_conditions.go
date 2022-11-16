package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/backend"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"

	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (p *Plugin) createRoutingNode(ctx context.Context, req *alertingv1.AttachedEndpoints, conditionId string) error {
	eList, err := p.ListAlertEndpoints(ctx, &alertingv1.ListAlertEndpointsRequest{})
	if err != nil {
		return err
	}
	routingNode, err := backend.ConvertEndpointIdsToRoutingNode(eList, req, conditionId)
	if err != nil {
		p.Logger.Error(err)
		return err
	}
	_, err = p.CreateConditionRoutingNode(ctx, routingNode)
	if err != nil {
		p.Logger.Error(err)
		return err
	}
	return nil
}

func (p *Plugin) updateRoutingNode(ctx context.Context, req *alertingv1.AttachedEndpoints, conditionId string) error {
	eList, err := p.ListAlertEndpoints(ctx, &alertingv1.ListAlertEndpointsRequest{})
	if err != nil {
		return err
	}
	routingNode, err := backend.ConvertEndpointIdsToRoutingNode(eList, req, conditionId)
	if err != nil {
		p.Logger.Error(err)
		return err
	}
	_, err = p.UpdateConditionRoutingNode(ctx, routingNode)
	if err != nil {
		p.Logger.Error(err)
		return err
	}
	return nil
}

func (p *Plugin) deleteRoutingNode(ctx context.Context, alertId string) error {
	_, err := p.DeleteConditionRoutingNode(ctx, &corev1.Reference{Id: alertId})
	if err != nil {
		p.Logger.Error(err)
		return err
	}
	return nil
}

func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1.AlertCondition) (*corev1.Reference, error) {
	lg := p.Logger.With("Handler", "CreateAlertCondition")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := alertingv1.DetailsHasImplementation(req.GetAlertType()); err != nil {
		return nil, shared.WithNotFoundError(fmt.Sprintf("%s", err))
	}
	newId := uuid.New().String()
	_, err := setupCondition(p, lg, ctx, req, newId)
	if err != nil {
		return nil, err
	}
	if alertingv1.ShouldCreateRoutingNode(nil, req.AttachedEndpoints) { //FIXME: this won't clean up setupCondition on failure
		err := p.createRoutingNode(ctx, req.AttachedEndpoints, newId)
		if err != nil {
			return nil, err
		}
	}
	if err := p.storageNode.CreateConditionStorage(ctx, newId, req); err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: newId}, nil
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertCondition, error) {
	return p.storageNode.GetConditionStorage(ctx, ref.Id)
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1.ListAlertConditionRequest) (*alertingv1.AlertConditionList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	keys, items, err := p.storageNode.ListWithKeyConditionStorage(ctx)
	if err != nil {
		return nil, err
	}

	res := &alertingv1.AlertConditionList{}
	for i := range keys {
		res.Items = append(res.Items, &alertingv1.AlertConditionWithId{
			Id:             &corev1.Reference{Id: keys[i]},
			AlertCondition: items[i],
		})
	}
	return res, nil
}

// req.Id is the condition id reference
func (p *Plugin) UpdateAlertCondition(ctx context.Context, req *alertingv1.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	lg := p.Logger.With("handler", "UpdateAlertCondition")
	lg.Debugf("Updating alert condition %s", req.Id)
	conditionId := req.Id.Id
	existing, err := p.storageNode.GetConditionStorage(ctx, req.Id.Id)
	if err != nil {
		return nil, err
	}

	_, err = setupCondition(p, lg, ctx, req.UpdateAlert, req.Id.Id)
	if err != nil {
		return nil, err
	}
	newAE, oldAE := req.UpdateAlert.AttachedEndpoints, existing.AttachedEndpoints
	if alertingv1.ShouldCreateRoutingNode(newAE, oldAE) { //FIXME: this won't clean up setupCondition on failure
		lg.Debugf("udpated condition %s must create an endpoint implementation", conditionId)
		err := p.createRoutingNode(ctx, newAE, conditionId)
		if err != nil {
			p.Logger.Errorf("creating routing node failed %s", err)
			return nil, err
		}
	} else if alertingv1.ShouldUpdateRoutingNode(newAE, oldAE) {
		lg.Debugf("udpated condition %s must update an existing endpoint implementation", conditionId)
		err := p.updateRoutingNode(ctx, newAE, conditionId)
		if err != nil {
			p.Logger.Errorf("updating routing node failed %s", err)
			return nil, err
		}
	} else if alertingv1.ShouldDeleteRoutingNode(newAE, oldAE) {
		lg.Debugf("udpated condition %s must delete an existing endpoint implementation", conditionId)
		err := p.deleteRoutingNode(ctx, conditionId)
		if err != nil {
			p.Logger.Errorf("deleting routing node failed %s", err)
			return nil, err
		}
	}
	if err := p.storageNode.UpdateConditionStorage(ctx, conditionId, req.UpdateAlert); err != nil {
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

	if alertingv1.ShouldDeleteRoutingNode(nil, existing.AttachedEndpoints) {
		lg.Debugf("Deleted condition %s must clean up its existing endpoint implementation", ref.Id)
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

func (p *Plugin) AlertConditionStatus(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertStatusResponse, error) {
	lg := p.Logger.With("handler", "AlertConditionStatus")
	lg.Debugf("Getting alert condition status %s", ref.Id)

	cond, err := p.storageNode.GetConditionStorage(ctx, ref.Id)
	if err != nil {
		lg.Errorf("failed to find condition with id %s in storage : %s", ref.Id, err)
		return nil, shared.WithNotFoundErrorf("%s", err)
	}

	if a := cond.GetAlertType().GetSystem(); a != nil {
		_, err := p.mgmtClient.Get().GetCluster(ctx, a.ClusterId)
		if err != nil {
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_INVALIDATED,
			}, nil
		}
	}

	if ref, _ := handleSwitchCortexRules(cond.GetAlertType()); ref != nil {
		// check monitoring backend is installed
		ctxca, ca := context.WithCancel(ctx)
		defer ca()
		cortexOpsClient, err := p.cortexOpsClient.GetContext(ctxca)
		if err != nil {
			return nil, err
		}
		backendStatus, err := cortexOpsClient.GetClusterStatus(ctx, &emptypb.Empty{})
		if err != nil {
			return nil, err
		}
		if backendStatus.State == cortexops.InstallState_NotInstalled {
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_INVALIDATED,
			}, nil
		}
		// check that monitoring is enabled on the cluster
		deets, err := p.mgmtClient.Get().GetCluster(ctx, ref)
		if err != nil {
			return nil, err
		}
		found := false
		for _, cap := range deets.GetCapabilities() {
			if cap.Name == "metrics" {
				found = true
			}
		}
		if !found {
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_INVALIDATED,
			}, nil
		}
	}

	defaultState := &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_OK,
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
	respAlertGroup := []backend.GettableAlert{}
	apiNode := backend.NewAlertManagerGetAlertsClient(
		ctx,
		availableEndpoint,
		backend.WithLogger(lg),
		backend.WithExpectClosure(func(resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code %d", resp.StatusCode)
			}

			return json.NewDecoder(resp.Body).Decode(&respAlertGroup)
		}))
	err = apiNode.DoRequest()
	if err != nil {
		return nil, err
	}
	if respAlertGroup == nil {
		return nil, shared.WithInternalServerError("cannot parse response body into expected api struct")
	}
	for _, alert := range respAlertGroup {
		for _, recv := range alert.Receivers {
			if recv.Name == nil {
				continue
			}
			if *recv.Name == ref.Id {
				if alert.Status.State == nil { // pretend everything is ok
					return defaultState, nil
				}
				switch *alert.Status.State {
				case models.AlertStatusStateSuppressed:
					return &alertingv1.AlertStatusResponse{
						State: alertingv1.AlertConditionState_SILENCED,
					}, nil
				case models.AlertStatusStateActive:
					return &alertingv1.AlertStatusResponse{
						State: alertingv1.AlertConditionState_FIRING,
					}, nil
				case models.AlertStatusStateUnprocessed:
					fallthrough
				default:
					return defaultState, nil
				}

			}
		}
	}
	return defaultState, nil
}

func (p *Plugin) ActivateSilence(ctx context.Context, req *alertingv1.SilenceRequest) (*emptypb.Empty, error) {
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
	if existing.Silence != nil && existing.Silence.SilenceId != "" {
		_, err := p.DeactivateSilence(ctx, &corev1.Reference{Id: req.ConditionId.Id})
		if err != nil {
			return nil, shared.WithInternalServerErrorf("failed to deactivate existing silence : %s", err)
		}
	}
	respSilence := &backend.PostSilencesResponse{}
	postSilenceNode := backend.NewAlertManagerPostSilenceClient(
		ctx,
		availableEndpoint,
		backend.WithLogger(p.Logger),
		backend.WithPostSilenceBody(req.ConditionId.Id, req.Duration.AsDuration(), silenceID),
		backend.WithExpectClosure(func(resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("failed to create silence : %s", resp.Status)
			}
			return json.NewDecoder(resp.Body).Decode(respSilence)
		}))
	err = postSilenceNode.DoRequest()
	if err != nil {
		p.Logger.Errorf("failed to post silence : %s", err)
		return nil, err
	}
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = &alertingv1.SilenceInfo{ // not exact, but the difference will be negligible
		SilenceId: respSilence.GetSilenceId(),
		StartsAt:  timestamppb.Now(),
		EndsAt:    timestamppb.New(time.Now().Add(req.Duration.AsDuration())),
	}
	// update K,V with new silence info for the respective condition
	if err := p.storageNode.UpdateConditionStorage(ctx, req.ConditionId.Id, newCondition); err != nil {
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
		ctx,
		availableEndpoint,
		existing.Silence.SilenceId,
		backend.WithLogger(p.Logger),
		backend.WithExpectClosure(backend.NewExpectStatusOk()))
	err = apiNode.DoRequest()
	if err != nil {
		p.Logger.Errorf("failed to delete silence : %s", err)
		return nil, err
	}

	// update existing proto with the silence info
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = nil
	// update K,V with new silence info for the respective condition
	if err := p.storageNode.UpdateConditionStorage(ctx, req.Id, newCondition); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) ListAlertConditionChoices(ctx context.Context, req *alertingv1.AlertDetailChoicesRequest) (*alertingv1.ListAlertTypeDetails, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := alertingv1.EnumHasImplementation(req.GetAlertType()); err != nil {
		return nil, err
	}
	return handleChoicesByType(p, ctx, req)
}

func (p *Plugin) Timeline(ctx context.Context, req *alertingv1.TimelineRequest) (*alertingv1.TimelineResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	ids, conditions, err := p.storageNode.ListWithKeyConditionStorage(ctx)
	if err != nil {
		return nil, err
	}
	resp := &alertingv1.TimelineResponse{
		Items: make(map[string]*alertingv1.ActiveWindows),
	}
	requiresCortex := false
	for idx, id := range ids {
		if k, _ := handleSwitchCortexRules(conditions[idx].GetAlertType()); k != nil {
			requiresCortex = true
		}
		resp.Items[id] = &alertingv1.ActiveWindows{
			Windows: make([]*alertingv1.ActiveWindow, 0),
		}
	}

	var cortexAdminClient cortexadmin.CortexAdminClient
	if requiresCortex {
		ctxCa, cancel := context.WithCancel(ctx)
		defer cancel()
		adminClient, err := p.adminClient.GetContext(ctxCa)
		if err != nil {
			return nil, util.StatusError(codes.Code(code.Code_FAILED_PRECONDITION))
		}
		cortexAdminClient = adminClient
	}

	start := timestamppb.New(time.Now().Add(-req.LookbackWindow.AsDuration()))
	end := timestamppb.Now()
	cortexStep := durationpb.New(req.LookbackWindow.AsDuration() / 500)
	var wg sync.WaitGroup
	var addMu sync.Mutex
	for idx := range conditions {
		idx := idx // capture in closure
		wg.Add(1)
		go func() {
			defer wg.Done()
			condition := conditions[idx]
			if s := condition.GetAlertType().GetSystem(); s != nil {
				// check system tracker
				activeWindows, err := p.storageNode.GetActiveWindowsFromAgentIncidentTracker(ctx, ids[idx], start, end)
				if err != nil {
					p.Logger.Errorf("failed to get active windows from agent incident tracker : %s", err)
					return
				}
				addMu.Lock()
				resp.Items[ids[idx]] = &alertingv1.ActiveWindows{
					Windows: activeWindows,
				}
				addMu.Unlock()
			}
			if r, info := handleSwitchCortexRules(condition.GetAlertType()); r != nil {
				qr, err := cortexAdminClient.QueryRange(ctx, &cortexadmin.QueryRangeRequest{
					Tenants: []string{r.Id},
					// Constructed recording rule, NOT alerting rule
					Query: fmt.Sprintf(
						"%s{%s}",
						ConstructRecordingRuleName(info.GoldenSignal(), info.AlertType()),
						ConstructFiltersFromMap(
							ConstructIdLabelsForRecordingRule(condition.Name, ids[idx]),
						),
					),
					Start: start,
					End:   end,
					Step:  cortexStep,
				})
				if err != nil {
					p.Logger.Errorf("failed to query cortex : %s", err)
					return
				}
				rawBytes := qr.Data
				qres, err := unmarshal.UnmarshalPrometheusResponse(rawBytes)
				if err != nil {
					p.Logger.Errorf("failed to unmarshal prometheus response : %s", err)
					return
				}
				dataMatrix, err := qres.GetMatrix()
				if err != nil {
					p.Logger.Errorf("failed to get matrix : %s", err)
					return
				}
				isRising := true
				isFiring := func(v model.SampleValue) bool {
					return v > 0
				}
				activeWindows := alertingv1.ActiveWindows{
					Windows: make([]*alertingv1.ActiveWindow, 0),
				}
				for _, row := range *dataMatrix {
					for _, rowValue := range row.Values {
						ts := time.Unix(rowValue.Timestamp.Unix(), 0)
						if !isFiring(rowValue.Value) && isRising {
							activeWindows.Windows = append(activeWindows.Windows, &alertingv1.ActiveWindow{
								Start: timestamppb.New(ts),
								End:   end,
								Type:  alertingv1.TimelineType_Timeline_Alerting,
							})
							isRising = false
						} else if isFiring(rowValue.Value) && !isRising {
							activeWindows.Windows[len(activeWindows.Windows)].End = timestamppb.New(ts)
							isRising = true
						}
					}
				}
				addMu.Lock()
				resp.Items[ids[idx]] = &activeWindows
				addMu.Unlock()
			}
		}()
	}
	wg.Wait()

	return resp, nil
}
