package alarms

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/alitto/pond"
	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"github.com/rancher/opni/plugins/alerting/apis/alertops"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var _ alertingv1.AlertConditionsServer = (*AlarmServerComponent)(nil)

func (a *AlarmServerComponent) CreateAlertCondition(ctx context.Context, req *alertingv1.AlertCondition) (*corev1.Reference, error) {
	lg := a.logger.With("Handler", "CreateAlertCondition")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	newId := shared.NewAlertingRefId()
	req.Id = newId
	req.LastUpdated = timestamppb.Now()
	if err := a.conditionStorage.Get().Put(ctx, newId, req); err != nil {
		return nil, err
	}
	status, err := a.opsNode.Get().GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if status.State != alertops.InstallState_Installed {
		return &corev1.Reference{Id: newId}, nil
	}
	if _, err := a.setupCondition(ctx, lg, req, newId); err != nil {
		return nil, err
	}
	return &corev1.Reference{Id: newId}, nil
}

func (a *AlarmServerComponent) GetAlertCondition(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertCondition, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	return a.conditionStorage.Get().Get(ctx, ref.Id)
}

func (a *AlarmServerComponent) ListAlertConditions(ctx context.Context, req *alertingv1.ListAlertConditionRequest) (*alertingv1.AlertConditionList, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	allConds, err := a.conditionStorage.Get().List(ctx)
	if err != nil {
		return nil, err
	}

	res := &alertingv1.AlertConditionList{}
	conds := lo.Filter(allConds, req.FilterFunc())
	for i := range conds {
		res.Items = append(res.Items, &alertingv1.AlertConditionWithId{
			Id:             &corev1.Reference{Id: conds[i].Id},
			AlertCondition: conds[i],
		})
	}
	slices.SortFunc(res.Items, func(a, b *alertingv1.AlertConditionWithId) bool {
		return a.AlertCondition.Name < b.AlertCondition.Name
	})
	return res, nil
}

func (a *AlarmServerComponent) UpdateAlertCondition(ctx context.Context, req *alertingv1.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	lg := a.logger.With("handler", "UpdateAlertCondition")
	lg.Debugf("Updating alert condition %s", req.Id)
	conditionId := req.Id.Id

	req.UpdateAlert.LastUpdated = timestamppb.Now()
	if err := a.conditionStorage.Get().Put(ctx, conditionId, req.UpdateAlert); err != nil {
		return nil, err
	}
	status, err := a.opsNode.Get().GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if status.State != alertops.InstallState_Installed {
		return &emptypb.Empty{}, nil
	}
	if _, err := a.setupCondition(ctx, lg, req.UpdateAlert, req.Id.Id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (a *AlarmServerComponent) DeleteAlertCondition(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	lg := a.logger.With("Handler", "DeleteAlertCondition")
	existing, err := a.conditionStorage.Get().Get(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	// this can happen if the condition is not in storage
	if existing == nil {
		return &emptypb.Empty{}, nil
	}

	if err := a.conditionStorage.Get().Delete(ctx, ref.Id); err != nil {
		return nil, err
	}
	status, err := a.opsNode.Get().GetClusterStatus(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	if status.State != alertops.InstallState_Installed {
		return &emptypb.Empty{}, nil
	}
	if err := a.deleteCondition(ctx, lg, existing, ref.Id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (a *AlarmServerComponent) ListAlertConditionChoices(ctx context.Context, req *alertingv1.AlertDetailChoicesRequest) (*alertingv1.ListAlertTypeDetails, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	return handleChoicesByType(ctx, a, req)
}

func (a *AlarmServerComponent) AlertConditionStatus(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertStatusResponse, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	lg := a.logger.With("handler", "AlertConditionStatus")

	// required info
	cond, err := a.conditionStorage.Get().Get(ctx, ref.Id)
	if err != nil {
		lg.Errorf("failed to find condition with id %s in storage : %s", ref.Id, err)
		return nil, shared.WithNotFoundErrorf("%s", err)
	}

	statusInfo, err := a.loadStatusInfo(ctx)
	if err != nil {
		return nil, err
	}

	resultStatus := &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}

	compareStatus := func(status *alertingv1.AlertStatusResponse) {
		if resultStatus.State < status.State {
			resultStatus.State = status.State
			resultStatus.Reason = status.Reason
		} else if resultStatus.State == alertingv1.AlertConditionState_Ok &&
			status.State == alertingv1.AlertConditionState_Unkown {
			resultStatus.State = status.State
			resultStatus.Reason = status.Reason
		}
	}

	compareStatus(a.checkClusterStatus(cond, *statusInfo))
	compareStatus(statusFromLoadedReceivers(
		cond,
		statusInfo.alertingInfo,
	))
	compareStatus(statusFromAlertGroup(
		cond,
		statusInfo.alertingInfo,
	))
	return resultStatus, nil
}

func (a *AlarmServerComponent) ListAlertConditionsWithStatus(ctx context.Context, req *alertingv1.ListStatusRequest) (*alertingv1.ListStatusResponse, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	allConds, err := a.conditionStorage.Get().List(ctx)
	if err != nil {
		return nil, err
	}
	res := &alertingv1.ListStatusResponse{
		AlertConditions: make(map[string]*alertingv1.AlertConditionWithStatus),
	}
	statusInfo, err := a.loadStatusInfo(ctx)
	if err != nil {
		return nil, err
	}
	conds := lo.Filter(allConds, func(item *alertingv1.AlertCondition, _ int) bool {
		if req.ItemFilter == nil {
			return true
		}
		if len(req.ItemFilter.Clusters) != 0 {
			if !slices.Contains(req.ItemFilter.Clusters, item.GetClusterId().Id) {
				return false
			}
		}
		if len(req.ItemFilter.Labels) != 0 {
			if len(lo.Intersect(req.ItemFilter.Labels, item.Labels)) != len(req.ItemFilter.Labels) {
				return false
			}
		}
		if len(req.ItemFilter.Severities) != 0 {
			if !slices.Contains(req.ItemFilter.Severities, item.Severity) {
				return false
			}
		}
		if len(req.ItemFilter.Severities) != 0 {
			matches := false
			for _, typ := range req.ItemFilter.GetAlertTypes() {
				if item.IsType(typ) {
					matches = true
					break
				}
			}
			if !matches {
				return false
			}
		}
		return true
	})
	for _, cond := range conds {

		resultStatus := &alertingv1.AlertStatusResponse{
			State: alertingv1.AlertConditionState_Ok,
		}
		compareStatus := func(status *alertingv1.AlertStatusResponse) {
			if resultStatus.State < status.State {
				resultStatus.State = status.State
				resultStatus.Reason = status.Reason
			} else if resultStatus.State == alertingv1.AlertConditionState_Ok &&
				status.State == alertingv1.AlertConditionState_Unkown {
				resultStatus.State = status.State
				resultStatus.Reason = status.Reason
			}
		}
		// do:
		// |
		// |--- check cluster configuration based on type
		// |    |
		// |    | check cluster dependencies status
		// |
		// | --- check router to get labels
		// |     |
		// |     | --- check receivers for that router are loaded
		// |     |
		// |	 | --- check alertmanager alert status for given labels

		compareStatus(a.checkClusterStatus(cond, *statusInfo))
		compareStatus(statusFromLoadedReceivers(
			cond,
			statusInfo.alertingInfo,
		))
		compareStatus(statusFromAlertGroup(
			cond,
			statusInfo.alertingInfo,
		))

		if len(req.States) != 0 {
			if !slices.Contains(req.States, resultStatus.State) {
				continue
			}
		}
		res.AlertConditions[cond.Id] = &alertingv1.AlertConditionWithStatus{
			AlertCondition: cond,
			Status:         resultStatus,
		}
	}
	return res, nil
}

func (a *AlarmServerComponent) CloneTo(ctx context.Context, req *alertingv1.CloneToRequest) (*emptypb.Empty, error) {
	lg := a.logger.With("handler", "CloneTo")
	if err := req.Validate(); err != nil {
		return nil, err
	}
	clusterLookup := map[string]struct{}{}
	cl, err := a.mgmtClient.Get().ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
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
	iErrGroup := &util.MultiErrGroup{}
	iErrGroup.Add(len(req.ToClusters))
	for _, ref := range req.ToClusters {
		ref := ref // capture in closure
		go func() {
			defer iErrGroup.Done()
			cond := util.ProtoClone(req.AlertCondition)
			cond.SetClusterId(&corev1.Reference{Id: ref})
			_, err := a.CreateAlertCondition(ctx, cond)
			if err != nil {
				lg.Errorf("failed to create alert condition %s", err)
				iErrGroup.AddError(err)
			}
		}()
	}
	iErrGroup.Wait()
	return &emptypb.Empty{}, iErrGroup.Error()
}

func (a *AlarmServerComponent) ActivateSilence(ctx context.Context, req *alertingv1.SilenceRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	existing, err := a.conditionStorage.Get().Get(ctx, req.ConditionId.Id)
	if err != nil {
		return nil, err
	}
	var silenceID *string
	if existing.Silence != nil {
		silenceID = &existing.Silence.SilenceId
	}
	if existing.Silence != nil && existing.Silence.SilenceId != "" {
		_, err := a.DeactivateSilence(ctx, &corev1.Reference{Id: req.ConditionId.Id})
		if err != nil {
			return nil, shared.WithInternalServerErrorf("failed to deactivate existing silence : %s", err)
		}
	}
	newId, err := a.Client.PostSilence(ctx, req.ConditionId.Id, req.Duration.AsDuration(), silenceID)
	// respSilence := &backend.PostSilencesResponse{}
	// postSilenceNode := backend.NewAlertManagerPostSilenceClient(
	// 	ctx,
	// 	availableEndpoint,
	// 	backend.WithLogger(a.logger),
	// 	backend.WithPostSilenceBody(req.ConditionId.Id, req.Duration.AsDuration(), silenceID),
	// 	backend.WithExpectClosure(func(resp *http.Response) error {
	// 		if resp.StatusCode != http.StatusOK {
	// 			return fmt.Errorf("failed to create silence : %s", resp.Status)
	// 		}
	// 		return json.NewDecoder(resp.Body).Decode(respSilence)
	// 	}))
	// err = postSilenceNode.DoRequest()
	// if err != nil {
	// 	a.logger.Errorf("failed to post silence : %s", err)
	// 	return nil, err
	// }
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = &alertingv1.SilenceInfo{ // not exact, but the difference will be negligible
		SilenceId: newId,
		StartsAt:  timestamppb.Now(),
		EndsAt:    timestamppb.New(time.Now().Add(req.Duration.AsDuration())),
	}
	// update K,V with new silence info for the respective condition
	if err := a.conditionStorage.Get().Put(ctx, req.ConditionId.Id, newCondition); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (a *AlarmServerComponent) DeactivateSilence(ctx context.Context, ref *corev1.Reference) (*emptypb.Empty, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	// options, err := a.opsNode.Get().GetRuntimeOptions(ctx)
	// if err != nil {
	// 	return nil, err
	// }
	existing, err := a.conditionStorage.Get().Get(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	if existing.Silence == nil {
		return nil, validation.Errorf("could not find existing silence for condition %s", ref.Id)
	}
	// availableEndpoint, err := a.opsNode.Get().GetAvailableEndpoint(ctx, &options)
	// if err != nil {
	// 	return nil, err
	// }
	if err := a.Client.DeleteSilence(ctx, existing.Silence.SilenceId); err != nil {
		return nil, err
	}

	// apiNode := backend.NewAlertManagerDeleteSilenceClient(
	// 	ctx,
	// 	availableEndpoint,
	// 	existing.Silence.SilenceId,
	// 	backend.WithLogger(a.logger),
	// 	backend.WithExpectClosure(backend.NewExpectStatusOk()))
	// err = apiNode.DoRequest()
	// if err != nil {
	// 	a.logger.Errorf("failed to delete silence : %s", err)
	// 	return nil, err
	// }

	// update existing proto with the silence info
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = nil
	// update K,V with new silence info for the respective condition
	if err := a.conditionStorage.Get().Put(ctx, ref.Id, newCondition); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (a *AlarmServerComponent) Timeline(ctx context.Context, req *alertingv1.TimelineRequest) (*alertingv1.TimelineResponse, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	lg := a.logger.With("handler", "Timeline")
	conditions, err := a.conditionStorage.Get().List(ctx)
	if err != nil {
		return nil, err
	}
	conds := lo.Filter(conditions, req.Filters.FilterFunc())
	resp := &alertingv1.TimelineResponse{
		Items: make(map[string]*alertingv1.ActiveWindows),
	}
	start := timestamppb.New(time.Now().Add(-req.LookbackWindow.AsDuration()))
	end := timestamppb.Now()
	ctxTimeout, ca := context.WithTimeout(ctx, 5*time.Second)
	defer ca()
	adminClient, err := a.adminClient.GetContext(ctxTimeout)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	yieldedValues := make(chan lo.Tuple2[string, *alertingv1.ActiveWindows])
	go func() {
		n := int(req.Limit)
		pool := pond.New(int(math.Max(25, float64(n/10))), n, pond.Strategy(pond.Balanced()))
		for _, cond := range conds {
			cond := cond
			pool.Submit(func() {
				if alertingv1.IsInternalCondition(cond) {
					activeWindows, err := a.incidentStorage.Get().GetActiveWindowsFromIncidentTracker(ctx, cond.Id, start, end)
					if err != nil {
						a.logger.Errorf("failed to get active windows from agent incident tracker : %s", err)
						return
					}
					yieldedValues <- lo.Tuple2[string, *alertingv1.ActiveWindows]{A: cond.Id, B: &alertingv1.ActiveWindows{
						Windows: activeWindows,
					}}
				}

				if alertingv1.IsMetricsCondition(cond) {
					res, err := adminClient.QueryRange(ctx, &cortexadmin.QueryRangeRequest{
						Tenants: []string{cond.GetClusterId().GetId()},
						Query:   fmt.Sprintf("ALERTS_FOR_STATE{opni_uuid=\"%s\"}", cond.Id),
						Start:   start,
						End:     end,
						Step:    durationpb.New(time.Minute * 1),
					})
					if err != nil {
						lg.Errorf("failed to query active windows from cortex : %s", err)
						return
					}
					qr, err := compat.UnmarshalPrometheusResponse(res.Data)
					if err != nil {
						lg.Errorf("failed to unmarshal prometheus response : %s", err)
						return
					}
					matrix, err := qr.GetMatrix()
					if err != nil || matrix == nil {
						lg.Errorf("expected to get matrix from prometheus response : %s", err)
						return
					}
					lg.With("reduce-matrix", cond.Id).Infof("looking to reduce %d potential causes", len(*matrix))
					yieldedValues <- lo.Tuple2[string, *alertingv1.ActiveWindows]{A: cond.Id, B: &alertingv1.ActiveWindows{
						Windows: cortex.ReducePrometheusMatrix(matrix),
					}}
				}
			})
		}
		pool.StopAndWait()
		close(yieldedValues)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case v, ok := <-yieldedValues:
			if !ok {
				return resp, nil
			}
			resp.Items[v.A] = v.B
		}
	}
}
