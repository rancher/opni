package alarms

import (
	"context"
	"fmt"
	"strings"
	"time"

	"slices"

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
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	_ alertingv1.AlertConditionsServer = (*AlarmServerComponent)(nil)
)

func (a *AlarmServerComponent) ListAlertConditionGroups(ctx context.Context, _ *emptypb.Empty) (*corev1.ReferenceList, error) {
	groups, err := a.conditionStorage.Get().ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	return &corev1.ReferenceList{
		Items: lo.Map(groups, func(item string, _ int) *corev1.Reference {
			return &corev1.Reference{Id: item}
		}),
	}, nil
}

func (a *AlarmServerComponent) CreateAlertCondition(ctx context.Context, req *alertingv1.AlertCondition) (*alertingv1.ConditionReference, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	newId := shared.NewAlertingRefId()
	req.Id = newId
	if req.Metadata == nil {
		req.Metadata = make(map[string]string)
	}
	req.LastUpdated = timestamppb.Now()
	if req.GetMetadata() == nil {
		req.Metadata = make(map[string]string)
	}
	req.Metadata[metadataInactiveAlarm] = "true"
	if err := a.conditionStorage.Get().Group(req.GroupId).Put(ctx, newId, req); err != nil {
		return nil, err
	}
	return &alertingv1.ConditionReference{Id: newId, GroupId: req.GroupId}, nil
}

func (a *AlarmServerComponent) GetAlertCondition(ctx context.Context, ref *alertingv1.ConditionReference) (*alertingv1.AlertCondition, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	return a.conditionStorage.Get().Group(ref.GroupId).Get(ctx, ref.Id)
}

func (a *AlarmServerComponent) ListAlertConditions(ctx context.Context, req *alertingv1.ListAlertConditionRequest) (*alertingv1.AlertConditionList, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	groupIds := []string{}
	if len(req.GroupIds) == 0 {
		groupIds = append(groupIds, "")
	} else {
		groupIds = req.GroupIds
	}
	allConds := []*alertingv1.AlertCondition{}

	for _, groupId := range groupIds {
		conds, err := a.conditionStorage.Get().Group(groupId).List(ctx)
		if err != nil {
			return nil, err
		}
		allConds = append(allConds, lo.Filter(conds, req.FilterFunc())...)
	}

	res := &alertingv1.AlertConditionList{}
	for i := range allConds {
		res.Items = append(res.Items, &alertingv1.AlertConditionWithId{
			Id:             &alertingv1.ConditionReference{Id: allConds[i].Id, GroupId: allConds[i].GroupId},
			AlertCondition: allConds[i],
		})
	}
	slices.SortFunc(res.Items, func(a, b *alertingv1.AlertConditionWithId) int {
		return strings.Compare(a.AlertCondition.Name, b.AlertCondition.Name)
	})
	return res, nil
}

func (a *AlarmServerComponent) UpdateAlertCondition(ctx context.Context, req *alertingv1.UpdateAlertConditionRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	lg := a.logger.With("handler", "UpdateAlertCondition")
	lg.Debugf("Updating alert condition %s", req.Id)
	conditionStorage := a.conditionStorage.Get()
	conditionId := req.Id.Id
	existingGroup := req.Id.GroupId
	incomingGroup := req.UpdateAlert.GroupId

	req.UpdateAlert.LastUpdated = timestamppb.Now()
	existing, err := conditionStorage.Group(existingGroup).Get(ctx, conditionId)
	if err != nil {
		return nil, err
	}
	if existing.GetMetadata() != nil && existing.GetMetadata()[metadataReadOnly] != "" {
		configurationChangeReadOnly := util.ProtoClone(existing)
		applyMutableReadOnlyFields(configurationChangeReadOnly, req.UpdateAlert)
		req.UpdateAlert = configurationChangeReadOnly
	}
	if existingGroup != incomingGroup {
		if err := conditionStorage.Group(existingGroup).Delete(ctx, conditionId); err != nil {
			return nil, err
		}
	}

	if err := a.conditionStorage.Get().Group(incomingGroup).Put(ctx, conditionId, req.UpdateAlert); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (a *AlarmServerComponent) DeleteAlertCondition(ctx context.Context, ref *alertingv1.ConditionReference) (*emptypb.Empty, error) {
	existing, err := a.conditionStorage.Get().Group(ref.GroupId).Get(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return &emptypb.Empty{}, nil
	}

	existing.GetMetadata()[metadataCleanUpAlarm] = "true"
	if err := a.conditionStorage.Get().Group(ref.GroupId).Put(ctx, ref.Id, existing); err != nil {
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

func (a *AlarmServerComponent) AlertConditionStatus(ctx context.Context, ref *alertingv1.ConditionReference) (*alertingv1.AlertStatusResponse, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	lg := a.logger.With("handler", "AlertConditionStatus")

	// required info
	cond, err := a.conditionStorage.Get().Group(ref.GroupId).Get(ctx, ref.Id)
	if err != nil {
		lg.Errorf("failed to find condition with id %s in storage : %s", ref.Id, err)
		return nil, shared.WithNotFoundErrorf("%s", err)
	}
	if cond.GetMetadata() != nil && cond.GetMetadata()[metadataInactiveAlarm] != "" {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "Alarm is scheduled for activation",
		}, nil
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

func (a *AlarmServerComponent) compareStatusInformation(cond *alertingv1.AlertCondition, statusInfo *statusInfo) *alertingv1.AlertStatusResponse {
	resultStatus := &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
	if cond.GetMetadata() != nil && cond.GetMetadata()[metadataInactiveAlarm] != "" {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "Alarm is scheduled for activation",
		}
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

	return resultStatus
}

func (a *AlarmServerComponent) ListAlertConditionsWithStatus(ctx context.Context, req *alertingv1.ListStatusRequest) (*alertingv1.ListStatusResponse, error) {
	if !a.Initialized() {
		return nil, status.Error(codes.Unavailable, "Alarm server is not yet available")
	}
	groupIds := []string{}
	if req.ItemFilter == nil || len(req.ItemFilter.GroupIds) == 0 {
		groupIds = append(groupIds, "")
	} else {
		groupIds = req.ItemFilter.GroupIds
	}
	allConds := []*alertingv1.AlertCondition{}
	for _, groupId := range groupIds {
		conds, err := a.conditionStorage.Get().Group(groupId).List(ctx)
		if err != nil {
			return nil, err
		}
		if req.ItemFilter != nil {
			allConds = append(allConds, lo.Filter(conds, req.ItemFilter.FilterFunc())...)
		} else {
			allConds = append(allConds, conds...)
		}
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

		resultStatus := a.compareStatusInformation(cond, statusInfo)

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
	eg := &util.MultiErrGroup{}
	for _, ref := range req.ToClusters {
		ref := ref // capture in closure
		eg.Go(func() error {
			cond := util.ProtoClone(req.AlertCondition)
			cond.SetClusterId(&corev1.Reference{Id: ref})
			_, err := a.CreateAlertCondition(ctx, cond)
			if err != nil {
				lg.Errorf("failed to create alert condition %s", err)
			}
			return err
		})
	}
	eg.Wait()
	return &emptypb.Empty{}, eg.Error()
}

func (a *AlarmServerComponent) ActivateSilence(ctx context.Context, req *alertingv1.SilenceRequest) (*emptypb.Empty, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	existing, err := a.conditionStorage.Get().Group(req.ConditionId.GroupId).Get(ctx, req.ConditionId.Id)
	if err != nil {
		return nil, err
	}

	var silenceID *string
	if existing.Silence != nil {
		silenceID = &existing.Silence.SilenceId
	}
	if existing.Silence != nil && existing.Silence.SilenceId != "" {
		_, err := a.DeactivateSilence(ctx, &alertingv1.ConditionReference{Id: req.ConditionId.Id, GroupId: req.ConditionId.GroupId})
		if err != nil {
			return nil, shared.WithInternalServerErrorf("failed to deactivate existing silence : %s", err)
		}
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	newId, err := a.Client.SilenceClient().PostSilence(ctx, req.ConditionId.Id, req.Duration.AsDuration(), silenceID)
	if err != nil {
		return nil, err
	}
	newCondition := util.ProtoClone(existing)
	newCondition.Silence = &alertingv1.SilenceInfo{ // not exact, but the difference will be negligible
		SilenceId: newId,
		StartsAt:  timestamppb.Now(),
		EndsAt:    timestamppb.New(time.Now().Add(req.Duration.AsDuration())),
	}
	// update K,V with new silence info for the respective condition
	if err := a.conditionStorage.Get().Group(req.ConditionId.GroupId).Put(ctx, req.ConditionId.Id, newCondition); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (a *AlarmServerComponent) DeactivateSilence(ctx context.Context, ref *alertingv1.ConditionReference) (*emptypb.Empty, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	existing, err := a.conditionStorage.Get().Group(ref.GroupId).Get(ctx, ref.Id)
	if err != nil {
		return nil, err
	}
	if existing.Silence == nil {
		return nil, validation.Errorf("could not find existing silence for condition %s", ref.Id)
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if err := a.Client.SilenceClient().DeleteSilence(ctx, existing.Silence.SilenceId); err != nil {
		return nil, err
	}

	newCondition := util.ProtoClone(existing)
	newCondition.Silence = nil
	// update K,V with new silence info for the respective condition
	if err := a.conditionStorage.Get().Group(ref.GroupId).Put(ctx, ref.Id, newCondition); err != nil {
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
	conditions := []*alertingv1.AlertCondition{}
	var groupIds []string
	if req.Filters == nil || len(req.Filters.GroupIds) == 0 {
		groupIds = []string{""}
	} else {
		groupIds = req.Filters.GroupIds
	}
	for _, groupId := range groupIds {
		conds, err := a.conditionStorage.Get().Group(groupId).List(ctx)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, lo.Filter(conds, req.Filters.FilterFunc())...)
	}

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
	yieldedValues := make(chan lo.Tuple2[*alertingv1.ConditionReference, *alertingv1.ActiveWindows])
	go func() {
		n := int(req.Limit)
		pool := pond.New(max(25, n/10), n, pond.Strategy(pond.Balanced()))
		for _, cond := range conditions {
			cond := cond
			ref := &alertingv1.ConditionReference{
				Id:      cond.Id,
				GroupId: cond.GroupId,
			}
			pool.Submit(func() {
				if alertingv1.IsInternalCondition(cond) {
					activeWindows, err := a.incidentStorage.Get().GetActiveWindowsFromIncidentTracker(ctx, cond.Id, start, end)
					if err != nil {
						a.logger.Errorf("failed to get active windows from agent incident tracker : %s", err)
						return
					}
					for _, w := range activeWindows {
						w.Ref = ref
					}
					yieldedValues <- lo.T2(
						ref,
						&alertingv1.ActiveWindows{
							Windows: activeWindows,
						})
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
					windows := cortex.ReducePrometheusMatrix(matrix)
					for _, w := range windows {
						w.Ref = ref
					}
					lg.With("reduce-matrix", cond.Id).Infof("looking to reduce %d potential causes", len(*matrix))
					yieldedValues <- lo.T2(
						ref,
						&alertingv1.ActiveWindows{
							Windows: windows,
						},
					)
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
			id := v.A.Id
			resp.Items[id] = v.B
		}
	}
}
