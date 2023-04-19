package alerting

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/alitto/pond"
	promClient "github.com/prometheus/client_golang/api/prometheus/v1"

	"github.com/rancher/opni/pkg/alerting/drivers/backend"
	"github.com/rancher/opni/pkg/alerting/drivers/cortex"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/metrics/compat"
	"github.com/rancher/opni/plugins/alerting/pkg/apis/alertops"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"

	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	if _, err := p.setupCondition(ctx, lg, req, newId); err != nil {
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
	allConds, err := p.storageClientSet.Get().Conditions().List(ctx)
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
	if _, err := p.setupCondition(ctx, lg, req.UpdateAlert, req.Id.Id); err != nil {
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
	if err := p.deleteCondition(ctx, lg, existing, ref.Id); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (p *Plugin) checkClusterStatus(cond *alertingv1.AlertCondition, info statusInfo) *alertingv1.AlertStatusResponse {
	clusterId := cond.GetClusterId()
	if clusterId == nil {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Unkown,
			Reason: "cluster id is not known at this time",
		}
	}
	if alertingv1.IsInternalCondition(cond) {
		return p.checkInternalClusterStatus(clusterId.Id, cond.Id, info.coreInfo)
	}
	if alertingv1.IsMetricsCondition(cond) {
		return p.checkMetricsClusterStatus(clusterId.Id, cond, info.coreInfo, info.metricsInfo)
	}
	return &alertingv1.AlertStatusResponse{
		State:  alertingv1.AlertConditionState_Unkown,
		Reason: "unknown condition type",
	}
}

func (p *Plugin) checkMetricsClusterStatus(
	clusterId string,
	cond *alertingv1.AlertCondition,
	coreInfo *coreInfo,
	metricsInfo *metricsInfo,
) *alertingv1.AlertStatusResponse {
	if metricsInfo.metricsBackendStatus.State == cortexops.InstallState_NotInstalled {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "metrics backend not installed",
		}
	}
	cluster, ok := coreInfo.clusterMap[clusterId]
	if !ok {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "cluster not found",
		}
	}
	if len(lo.Filter(cluster.Metadata.Capabilities, func(cap *corev1.ClusterCapability, _ int) bool {
		return cap.Name == wellknown.CapabilityMetrics
	})) == 0 {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "cluster does not have metrics capabilities installed",
		}
	}
	if !cond.GetLastUpdated().AsTime().Before(time.Now().Add(-(60 * time.Second))) {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "alarm metric dependencies are updating",
		}
	}
	if status := evaluatePrometheusRuleHealth(metricsInfo.cortexRules, cond.GetId()); status != nil {
		return status
	}
	return &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
}

func (p *Plugin) checkInternalClusterStatus(clusterId, conditionId string, coreInfo *coreInfo) *alertingv1.AlertStatusResponse {
	_, ok := coreInfo.clusterMap[clusterId]
	if clusterId != alertingv1.UpstreamClusterId && !ok {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "cluster not found",
		}
	}
	if !p.msgNode.IsRunning(conditionId) {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "internal server error -- restart gateway",
		}
	}
	return &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
}

func (p *Plugin) loadCoreInfo(ctx context.Context) (*coreInfo, error) {
	ctxCa, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	mgmtClient, err := p.mgmtClient.GetContext(ctxCa)
	if err != nil {
		return nil, err
	}
	clusterList, err := mgmtClient.ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
	if err != nil {
		return nil, err
	}
	clMap := clusterMap{}
	for _, cl := range clusterList.Items {
		clMap[cl.Id] = cl
	}
	return &coreInfo{
		clusterMap: clMap,
	}, nil
}

func (p *Plugin) loadAlertingInfo(ctx context.Context) (*alertingInfo, error) {
	router, err := p.storageClientSet.Get().Routers().Get(ctx, shared.SingleConfigId)
	if err != nil {
		return nil, err
	}

	options, err := p.opsNode.GetRuntimeOptions(ctx)
	if err != nil {
		return nil, err
	}
	// FIXME: the alert status returned by this endpoint
	// will not always be consistent within the HA vanilla AlertManager,
	// move this logic to cortex AlertManager member set when applicable
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, &options)
	if err != nil {
		return nil, err
	}

	respAlertGroup := []backend.GettableAlert{}
	apiNodeGetAlerts := backend.NewAlertManagerGetAlertsClient(
		ctx,
		availableEndpoint,
		backend.WithExpectClosure(func(resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code %d", resp.StatusCode)
			}

			return json.NewDecoder(resp.Body).Decode(&respAlertGroup)
		}))
	err = apiNodeGetAlerts.DoRequest()
	if err != nil {
		return nil, err
	}
	if respAlertGroup == nil {
		return nil, shared.WithInternalServerError("cannot parse response body into expected api struct")
	}

	respReceiver := []backend.Receiver{}
	apiNodeGetReceivers := backend.NewAlertManagerReceiversClient(
		ctx,
		availableEndpoint,
		backend.WithExpectClosure(func(resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code %d", resp.StatusCode)
			}
			return json.NewDecoder(resp.Body).Decode(&respReceiver)
		}))
	err = apiNodeGetReceivers.DoRequest()
	if err != nil {
		return nil, err
	}
	if respReceiver == nil {
		return nil, shared.WithInternalServerError("cannot parse response body into expected api struct")
	}
	allReceiverNames := lo.Map(respReceiver, func(r backend.Receiver, _ int) string {
		if r.Name == nil {
			return ""
		}
		return *r.Name
	})
	return &alertingInfo{
		router:          router,
		alertGroup:      respAlertGroup,
		loadedReceivers: allReceiverNames,
	}, nil
}

func (p *Plugin) loadMetricsInfo(ctx context.Context) (*metricsInfo, error) {
	mgmtClient, err := p.mgmtClient.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	clusterList, err := mgmtClient.ListClusters(
		caching.WithGrpcClientCaching(ctx, 1*time.Minute),
		&managementv1.ListClustersRequest{},
	)
	if err != nil {
		return nil, err
	}
	cortexOpsClient, err := p.cortexOpsClient.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	metricsBackendStatus, err := cortexOpsClient.GetClusterStatus(ctx, &emptypb.Empty{})
	if util.StatusCode(err) == codes.Unavailable || util.StatusCode(err) == codes.Unimplemented {
		metricsBackendStatus = &cortexops.InstallStatus{
			State: cortexops.InstallState_NotInstalled,
		}
	} else if err != nil {
		return nil, err
	}
	var crs *cortexadmin.RuleGroups
	if metricsBackendStatus.State == cortexops.InstallState_Installed {
		cortexAdminClient, err := p.adminClient.GetContext(ctx)
		if err != nil {
			return nil, err
		}
		ruleResp, err := cortexAdminClient.ListRules(ctx, &cortexadmin.ListRulesRequest{
			ClusterId: lo.Map(clusterList.Items, func(cl *corev1.Cluster, _ int) string {
				return cl.Id
			}),
			RuleType:        []string{string(promClient.RuleTypeAlerting)},
			NamespaceRegexp: shared.OpniAlertingCortexNamespace,
		})
		if err != nil {
			return nil, err
		}
		crs = ruleResp.Data
	}
	return &metricsInfo{
		metricsBackendStatus: metricsBackendStatus,
		cortexRules:          crs,
	}, nil
}

func (p *Plugin) loadStatusInfo(ctx context.Context) (*statusInfo, error) {
	// lg := p.Logger.With("request", "loadStatusInfo")

	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	eg, _ := errgroup.WithContext(ctxca)
	status := &statusInfo{
		mu: &sync.Mutex{},
	}
	eg.Go(
		func() error {
			info, err := p.loadCoreInfo(ctx)
			if err != nil {
				return err
			}
			status.setCoreInfo(info)
			return nil
		},
	)
	eg.Go(
		func() error {
			info, err := p.loadMetricsInfo(ctx)
			if err != nil {
				return err
			}
			status.setMetricsInfo(info)
			return nil
		},
	)
	eg.Go(
		func() error {
			info, err := p.loadAlertingInfo(ctx)
			if err != nil {
				return err
			}
			status.setAlertingInfo(info)
			return nil
		},
	)
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return status, nil
}

func (p *Plugin) AlertConditionStatus(ctx context.Context, ref *corev1.Reference) (*alertingv1.AlertStatusResponse, error) {
	lg := p.Logger.With("handler", "AlertConditionStatus")

	// required info
	cond, err := p.storageClientSet.Get().Conditions().Get(ctx, ref.Id)
	if err != nil {
		lg.Errorf("failed to find condition with id %s in storage : %s", ref.Id, err)
		return nil, shared.WithNotFoundErrorf("%s", err)
	}

	statusInfo, err := p.loadStatusInfo(ctx)
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

	compareStatus(p.checkClusterStatus(cond, *statusInfo))
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

func (p *Plugin) ListAlertConditionsWithStatus(ctx context.Context, req *alertingv1.ListStatusRequest) (*alertingv1.ListStatusResponse, error) {
	allConds, err := p.storageClientSet.Get().Conditions().List(ctx)
	if err != nil {
		return nil, err
	}
	res := &alertingv1.ListStatusResponse{
		AlertConditions: make(map[string]*alertingv1.AlertConditionWithStatus),
	}
	statusInfo, err := p.loadStatusInfo(ctx)
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

		compareStatus(p.checkClusterStatus(cond, *statusInfo))
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
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, &options)
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
	availableEndpoint, err := p.opsNode.GetAvailableEndpoint(ctx, &options)
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
	cl, err := p.mgmtClient.Get().ListClusters(
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
	return handleChoicesByType(ctx, p, req)
}

func (p *Plugin) Timeline(ctx context.Context, req *alertingv1.TimelineRequest) (*alertingv1.TimelineResponse, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	lg := p.Logger.With("handler", "Timeline")
	conditions, err := p.storageClientSet.Get().Conditions().List(ctx)
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
	adminClient, err := p.adminClient.GetContext(ctxTimeout)
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
					activeWindows, err := p.storageClientSet.Get().Incidents().GetActiveWindowsFromIncidentTracker(ctx, cond.Id, start, end)
					if err != nil {
						p.Logger.Errorf("failed to get active windows from agent incident tracker : %s", err)
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
