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
	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/metrics/unmarshal"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"golang.org/x/exp/slices"

	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

// always overwrites id field
func (p *Plugin) CreateAlertCondition(ctx context.Context, req *alertingv1.AlertCondition) (*corev1.Reference, error) {
	lg := p.Logger.With("Handler", "CreateAlertCondition")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := alertingv1.DetailsHasImplementation(req.GetAlertType()); err != nil {
		return nil, shared.WithNotFoundError(fmt.Sprintf("%s", err))
	}
	newId := shared.NewAlertingRefId()
	req.Id = newId
	req.LastUpdated = timestamppb.Now()
	if err := p.storageClientSet.Get().Conditions().Put(ctx, newId, req); err != nil {
		return nil, err
	}
	status, err := p.opsNode.GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if status.State != alertops.InstallState_Installed {
		return &corev1.Reference{Id: newId}, nil
	}
	if _, err := setupCondition(p, lg, ctx, req, newId); err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: newId}, nil
}

func (p *Plugin) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertCondition, error) {
	return p.storageClientSet.Get().Conditions().Get(ctx, ref.Id)
}

func (p *Plugin) ListAlertConditions(ctx context.Context, req *alertingv1.ListAlertConditionRequest) (*alertingv1.AlertConditionList, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	items, err := p.storageClientSet.Get().Conditions().List(ctx)
	if err != nil {
		return nil, err
	}

	res := &alertingv1.AlertConditionList{}
	for i := range items {
		res.Items = append(res.Items, &alertingv1.AlertConditionWithId{
			Id:             &corev1.Reference{Id: items[i].Id},
			AlertCondition: items[i],
		})
	}
	slices.SortFunc(res.Items, func(a, b *alertingv1.AlertConditionWithId) bool {
		return a.AlertCondition.Name < b.AlertCondition.Name
	})
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

	req.UpdateAlert.LastUpdated = timestamppb.Now()
	if err := p.storageClientSet.Get().Conditions().Put(ctx, conditionId, req.UpdateAlert); err != nil {
		return nil, err
	}
	status, err := p.opsNode.GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if status.State != alertops.InstallState_Installed {
		return &emptypb.Empty{}, nil
	}
	if _, err := setupCondition(p, lg, ctx, req.UpdateAlert, req.Id.Id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	lg := p.Logger.With("Handler", "DeleteAlertCondition")
	existing, err := p.storageClientSet.Get().Conditions().Get(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	// this can happen if the condition is not in storage
	if existing == nil {
		return &emptypb.Empty{}, nil
	}

	if err := p.storageClientSet.Get().Conditions().Delete(ctx, ref.Id); err != nil {
		return nil, err
	}
	status, err := p.opsNode.GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if status.State != alertops.InstallState_Installed {
		return &emptypb.Empty{}, nil
	}
	if err := deleteCondition(p, lg, ctx, existing, ref.Id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) AlertConditionStatus(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertStatusResponse, error) {
	lg := p.Logger.With("handler", "AlertConditionStatus")

	// required info
	cond, err := p.storageClientSet.Get().Conditions().Get(ctx, ref.Id)
	if err != nil {
		lg.Errorf("failed to find condition with id %s in storage : %s", ref.Id, err)
		return nil, shared.WithNotFoundErrorf("%s", err)
	}

	if cond.LastUpdated.AsTime().Unix() > time.Now().Add(-time.Minute*1).Unix() {
		return &alertingv1.AlertStatusResponse{
			State: alertingv1.AlertConditionState_Pending,
		}, nil
	}

	router, err := p.storageClientSet.Get().Routers().Get(ctx, shared.SingleConfigId)
	if err != nil {
		return nil, err
	}
	matchers := router.HasLabels(cond.Id)
	if matchers == nil {
		return &alertingv1.AlertStatusResponse{
			State: alertingv1.AlertConditionState_Pending,
		}, nil
	}

	if a := cond.GetAlertType().GetSystem(); a != nil {
		_, err := p.mgmtClient.Get().GetCluster(ctx, a.ClusterId)
		if err != nil || !p.msgNode.IsRunning(ref.Id) {
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_Invalidated,
			}, nil
		}
	}
	if dc := cond.GetAlertType().GetDownstreamCapability(); dc != nil {
		_, err := p.mgmtClient.Get().GetCluster(ctx, dc.ClusterId)
		if err != nil || !p.msgNode.IsRunning(ref.Id) {
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_Invalidated,
			}, nil
		}
	}
	if cc := cond.GetAlertType().GetMonitoringBackend(); cc != nil {
		if !p.msgNode.IsRunning(ref.Id) {
			return &alertingv1.AlertStatusResponse{
				State: alertingv1.AlertConditionState_Invalidated,
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
				State: alertingv1.AlertConditionState_Invalidated,
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
				State: alertingv1.AlertConditionState_Invalidated,
			}, nil
		}
	}

	defaultState := &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		lg.Errorf("Failed to get alerting options : %s", err)
		return nil, err
	}
	// FIXME: the alert status returned by this endpoint
	// will not always be consistent within the HA vanilla AlertManager,
	// move this logic to cortex AlertManager member set when applicable
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
		matched := false
		for labelName, label := range alert.Labels {
			for _, conditionMatcher := range matchers {
				if conditionMatcher.Name == labelName {
					if conditionMatcher.Matches(label) {
						matched = true
						break
					}
				}
			}
			if matched {
				switch *alert.Status.State {
				case models.AlertStatusStateSuppressed:
					return &alertingv1.AlertStatusResponse{
						State: alertingv1.AlertConditionState_Silenced,
					}, nil
				case models.AlertStatusStateActive:
					return &alertingv1.AlertStatusResponse{
						State: alertingv1.AlertConditionState_Firing,
					}, nil
				case models.AlertStatusStateUnprocessed: // in our case unprocessed means it has arrived for firing
					return &alertingv1.AlertStatusResponse{
						State: alertingv1.AlertConditionState_Firing,
					}, nil
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
	existing, err := p.storageClientSet.Get().Conditions().Get(ctx, req.ConditionId.Id)
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
	if err := p.storageClientSet.Get().Conditions().Put(ctx, req.ConditionId.Id, newCondition); err != nil {
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
	existing, err := p.storageClientSet.Get().Conditions().Get(ctx, req.Id)
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
	if err := p.storageClientSet.Get().Conditions().Put(ctx, req.Id, newCondition); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) CloneTo(ctx context.Context, req *alertingv1.CloneToRequest) (*emptypb.Empty, error) {
	lg := p.Logger.With("handler", "CloneTo")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	clusterLookup := map[string]struct{}{}
	cl, err := p.mgmtClient.Get().ListClusters(ctx, &managementv1.ListClustersRequest{})
	if err != nil {
		return nil, err
	}
	for _, c := range cl.Items {
		clusterLookup[c.Id] = struct{}{}
	}
	for _, ref := range req.ToClusters {
		if _, ok := clusterLookup[ref]; !ok {
			return nil, validation.Errorf("cluster could not be found %s", ref)
		}
	}
	iErrGroup := &independentErrGroup{}
	iErrGroup.Add(len(req.ToClusters))
	for _, ref := range req.ToClusters {
		ref := ref // capture in closure
		go func() {
			defer iErrGroup.Done()
			cond := util.ProtoClone(req.AlertCondition)
			cond.SetClusterId(&corev1.Reference{Id: ref})
			_, err := p.CreateAlertCondition(ctx, cond)
			if err != nil {
				lg.Errorf("failed to create alert condition %s", err)
				iErrGroup.AddError(err)
			}
		}()
	}
	iErrGroup.Wait()
	return &emptypb.Empty{}, iErrGroup.Error()
}

func (p *Plugin) ListAlertConditionChoices(ctx context.Context, req *alertingv1.AlertDetailChoicesRequest) (*alertingv1.ListAlertTypeDetails, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	if err := alertingv1.EnumHasImplementation(req.GetAlertType()); err != nil {
		return nil, err
	}
	return handleChoicesByType(ctx, p, req)
}

func (p *Plugin) Timeline(ctx context.Context, req *alertingv1.TimelineRequest) (*alertingv1.TimelineResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	conditions, err := p.storageClientSet.Get().Conditions().List(ctx)
	if err != nil {
		return nil, err
	}
	resp := &alertingv1.TimelineResponse{
		Items: make(map[string]*alertingv1.ActiveWindows),
	}
	requiresCortex := false
	for _, cond := range conditions {
		if k, _ := handleSwitchCortexRules(cond.GetAlertType()); k != nil {
			requiresCortex = true
		}
		resp.Items[cond.Id] = &alertingv1.ActiveWindows{
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
	for _, cond := range conditions {
		cond := cond
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s := cond.GetAlertType().GetSystem(); s != nil {
				// check system tracker
				activeWindows, err := p.storageClientSet.Get().Incidents().GetActiveWindowsFromIncidentTracker(ctx, cond.Id, start, end)
				if err != nil {
					p.Logger.Errorf("failed to get active windows from agent incident tracker : %s", err)
					return
				}
				addMu.Lock()
				resp.Items[cond.Id] = &alertingv1.ActiveWindows{
					Windows: activeWindows,
				}
				addMu.Unlock()
			}
			if dc := cond.GetAlertType().GetDownstreamCapability(); dc != nil {
				// check system tracker
				activeWindows, err := p.storageClientSet.Get().Incidents().GetActiveWindowsFromIncidentTracker(ctx, cond.Id, start, end)
				if err != nil {
					p.Logger.Errorf("failed to get active windows from agent incident tracker : %s", err)
					return
				}
				addMu.Lock()
				resp.Items[cond.Id] = &alertingv1.ActiveWindows{
					Windows: activeWindows,
				}
				addMu.Unlock()
			}
			if mb := cond.GetAlertType().GetMonitoringBackend(); mb != nil {
				// check system tracker
				activeWindows, err := p.storageClientSet.Get().Incidents().GetActiveWindowsFromIncidentTracker(ctx, cond.Id, start, end)
				if err != nil {
					p.Logger.Errorf("failed to get active windows from agent incident tracker : %s", err)
					return
				}
				addMu.Lock()
				resp.Items[cond.Id] = &alertingv1.ActiveWindows{
					Windows: activeWindows,
				}
				addMu.Unlock()
			}
			if r, info := handleSwitchCortexRules(cond.GetAlertType()); r != nil {
				qr, err := cortexAdminClient.QueryRange(ctx, &cortexadmin.QueryRangeRequest{
					Tenants: []string{r.Id},
					// Constructed recording rule, NOT alerting rule
					Query: fmt.Sprintf(
						"%s{%s}",
						cortex.ConstructRecordingRuleName(info.GoldenSignal(), info.AlertType()),
						cortex.ConstructFiltersFromMap(
							cortex.ConstructIdLabelsForRecordingRule(cond.Name, cond.Id),
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
				resp.Items[cond.Id] = &activeWindows
				addMu.Unlock()
			}
		}()
	}
	wg.Wait()

	return resp, nil
}
