package alarms

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
	promClient "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rancher/opni/pkg/alerting/drivers/routing"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/caching"
	"github.com/rancher/opni/pkg/capabilities"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/rancher/opni/pkg/plugins/driverutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/apis/cortexadmin"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (a *AlarmServerComponent) checkClusterStatus(cond *alertingv1.AlertCondition, info statusInfo) *alertingv1.AlertStatusResponse {
	if cond.GetMetadata() != nil && cond.GetMetadata()[metadataCleanUpAlarm] != "" {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Deleting,
			Reason: "Alarm is queued for deletion",
		}
	}
	clusterId := cond.GetClusterId()
	if clusterId == nil {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Unkown,
			Reason: "cluster id is not known at this time",
		}
	}
	if alertingv1.IsInternalCondition(cond) {
		return a.checkInternalClusterStatus(clusterId.Id, cond.Id, info.coreInfo)
	}
	if alertingv1.IsMetricsCondition(cond) {
		return a.checkMetricsClusterStatus(clusterId.Id, cond, info.coreInfo, info.metricsInfo)
	}
	return &alertingv1.AlertStatusResponse{
		State:  alertingv1.AlertConditionState_Unkown,
		Reason: "unknown condition type",
	}
}

func (a *AlarmServerComponent) checkMetricsClusterStatus(
	clusterId string,
	cond *alertingv1.AlertCondition,
	coreInfo *coreInfo,
	metricsInfo *metricsInfo,
) *alertingv1.AlertStatusResponse {
	if metricsInfo.metricsBackendStatus.InstallState == driverutil.InstallState_NotInstalled {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "metrics backend not installed",
		}
	}
	if metricsInfo.metricsBackendStatus.AppState != driverutil.ApplicationState_Running {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "metrics backend is not yet running",
		}
	}
	cluster, ok := coreInfo.clusterMap[clusterId]
	if !ok {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "cluster not found",
		}
	}
	if !capabilities.Has(cluster, capabilities.Cluster(wellknown.CapabilityMetrics)) {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "cluster does not have metrics capabilities installed",
		}
	}
	if status := evaluatePrometheusRuleHealth(metricsInfo.cortexRules, cond.GetId()); status != nil {
		return status
	}
	return &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
}

func (a *AlarmServerComponent) checkInternalClusterStatus(clusterId, conditionId string, coreInfo *coreInfo) *alertingv1.AlertStatusResponse {
	_, ok := coreInfo.clusterMap[clusterId]
	if clusterId != alertingv1.UpstreamClusterId && !ok {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "cluster not found",
		}
	}
	if !a.runner.IsRunning(conditionId) {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Invalidated,
			Reason: "internal server error -- restart gateway",
		}
	}
	return &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
}

func (a *AlarmServerComponent) loadStatusInfo(ctx context.Context) (*statusInfo, error) {
	ctxca, ca := context.WithTimeout(ctx, 10*time.Second)
	defer ca()
	eg, workCtx := errgroup.WithContext(ctxca)
	status := &statusInfo{
		mu: &sync.Mutex{},
	}
	eg.Go(
		func() error {
			info, err := a.loadCoreInfo(workCtx)
			if err != nil {
				return err
			}
			status.setCoreInfo(info)
			return nil
		},
	)
	eg.Go(
		func() error {
			info, err := a.loadMetricsInfo(workCtx)
			if err != nil {
				return err
			}
			status.setMetricsInfo(info)
			return nil
		},
	)
	eg.Go(
		func() error {
			info, err := a.loadAlertingInfo(workCtx)
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

func (a *AlarmServerComponent) loadCoreInfo(ctx context.Context) (*coreInfo, error) {
	ctxCa, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	mgmtClient, err := a.mgmtClient.GetContext(ctxCa)
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

func (a *AlarmServerComponent) loadAlertingInfo(ctx context.Context) (*alertingInfo, error) {
	router, err := a.routerStorage.Get().Get(ctx, shared.SingleConfigId)
	if err != nil {
		return nil, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	ags, err := a.Client.AlertClient().ListAlerts(ctx)
	if err != nil {
		return nil, err
	}
	respReceiver, err := a.Client.ConfigClient().ListReceivers(ctx)
	if err != nil {
		return nil, err
	}
	allReceiverNames := lo.Map(respReceiver, func(r models.Receiver, _ int) string {
		if r.Name == nil {
			return ""
		}
		return *r.Name
	})
	return &alertingInfo{
		router:          router,
		alertGroup:      ags,
		loadedReceivers: allReceiverNames,
	}, nil
}

func (a *AlarmServerComponent) loadMetricsInfo(ctx context.Context) (*metricsInfo, error) {
	mgmtClient, err := a.mgmtClient.GetContext(ctx)
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
	cortexOpsClient, err := a.cortexOpsClient.GetContext(ctx)
	if err != nil {
		return nil, err
	}
	metricsBackendStatus, err := cortexOpsClient.Status(ctx, &emptypb.Empty{})
	if util.StatusCode(err) == codes.Unavailable || util.StatusCode(err) == codes.Unimplemented {
		metricsBackendStatus = &driverutil.InstallStatus{}
	} else if err != nil {
		return nil, err
	}
	var crs *cortexadmin.RuleGroups
	if metricsBackendStatus.AppState == driverutil.ApplicationState_Running {
		cortexAdminClient, err := a.adminClient.GetContext(ctx)
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

type clusterMap map[string]*corev1.Cluster

type statusInfo struct {
	mu           *sync.Mutex
	coreInfo     *coreInfo
	metricsInfo  *metricsInfo
	alertingInfo *alertingInfo
}

func (s *statusInfo) setMetricsInfo(i *metricsInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metricsInfo = i
}

func (s *statusInfo) setCoreInfo(i *coreInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.coreInfo = i
}

func (s *statusInfo) setAlertingInfo(i *alertingInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.alertingInfo = i
}

type coreInfo struct {
	clusterMap clusterMap
}

type metricsInfo struct {
	metricsBackendStatus *driverutil.InstallStatus
	cortexRules          *cortexadmin.RuleGroups
}

type alertingInfo struct {
	router          routing.OpniRouting
	alertGroup      models.AlertGroups
	loadedReceivers []string
}

func statusFromLoadedReceivers(
	cond *alertingv1.AlertCondition,
	alertInfo *alertingInfo,
) *alertingv1.AlertStatusResponse {
	matchers := alertInfo.router.HasLabels(cond.Id)
	requiredReceivers := alertInfo.router.HasReceivers(cond.Id)

	if len(requiredReceivers) == 0 ||
		cond.AttachedEndpoints == nil ||
		len(cond.AttachedEndpoints.Items) == 0 {
		return &alertingv1.AlertStatusResponse{
			State: alertingv1.AlertConditionState_Ok,
		}
	}
	matchingReceivers := lo.Intersect(requiredReceivers, alertInfo.loadedReceivers)
	if len(matchers) != 0 && len(matchingReceivers) != len(requiredReceivers) {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "configuration updates are scheduled for activation",
		}
	}
	return &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
}

func statusFromAlertGroup(
	cond *alertingv1.AlertCondition,
	alertInfo *alertingInfo,
) *alertingv1.AlertStatusResponse {
	matchers := alertInfo.router.HasLabels(cond.Id)
	requiredReceivers := alertInfo.router.HasReceivers(cond.Id)
	alertGroups := alertInfo.alertGroup
	defaultState := &alertingv1.AlertStatusResponse{
		State: alertingv1.AlertConditionState_Ok,
	}
	if len(requiredReceivers) == 0 && cond.AttachedEndpoints != nil && len(cond.AttachedEndpoints.Items) > 0 {
		defaultState = &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "configuration updates are scheduled for activation",
		}
	}
	for _, group := range alertGroups {
		for _, alert := range group.Alerts {
			// must match all matchers from the router spec to the alert's labels
			if !lo.EveryBy(matchers, func(m *labels.Matcher) bool {
				for labelName, label := range alert.Labels {
					if m.Name == labelName && m.Matches(label) {
						return true
					}
				}
				return false
			}) {
				continue // these are not the alerts you are looking for
			}
			switch *alert.Status.State {
			case models.AlertStatusStateSuppressed:
				return &alertingv1.AlertStatusResponse{
					State: alertingv1.AlertConditionState_Silenced,
				}
			case models.AlertStatusStateActive:
				return &alertingv1.AlertStatusResponse{
					State: alertingv1.AlertConditionState_Firing,
				}
			case models.AlertStatusStateUnprocessed:
				// in our case unprocessed means it has arrived for firing
				return &alertingv1.AlertStatusResponse{
					State: alertingv1.AlertConditionState_Firing,
				}
			default:
				return &alertingv1.AlertStatusResponse{
					State: alertingv1.AlertConditionState_Ok,
				}
			}
		}
	}
	return defaultState
}

func evaluatePrometheusRuleHealth(ruleList *cortexadmin.RuleGroups, id string) *alertingv1.AlertStatusResponse {
	if ruleList == nil {
		return &alertingv1.AlertStatusResponse{
			State:  alertingv1.AlertConditionState_Pending,
			Reason: "waiting for monitoring rule state(s) to be available from metrics backend",
		}
	}

	for _, group := range ruleList.GetGroups() {
		if strings.Contains(group.GetName(), id) {
			if len(group.GetRules()) == 0 {
				return &alertingv1.AlertStatusResponse{
					State:  alertingv1.AlertConditionState_Pending,
					Reason: "waiting for monitoring rule state(s) to be available from metrics backend",
				}
			}
			healthList := lo.Map(group.GetRules(), func(rule *cortexadmin.Rule, _ int) string {
				return rule.GetHealth()
			})
			health := lo.Associate(healthList, func(health string) (string, struct{}) {
				return health, struct{}{}
			})
			if _, ok := health[promClient.RuleHealthBad]; ok {
				return &alertingv1.AlertStatusResponse{
					State:  alertingv1.AlertConditionState_Invalidated,
					Reason: "one or more metric dependencies are unable to be evaluated",
				}
			}
			if _, ok := health[promClient.RuleHealthUnknown]; ok {
				return &alertingv1.AlertStatusResponse{
					State:  alertingv1.AlertConditionState_Pending,
					Reason: "waiting for monitoring rule state(s) to be available from metrics backend",
				}
			}
		}
	}
	return nil
}
