package alerting

// import (
// 	"fmt"
// 	"sort"
// 	"strings"
// 	"sync"

// 	"github.com/prometheus/alertmanager/api/v2/models"
// 	"github.com/prometheus/alertmanager/pkg/labels"
// 	promClient "github.com/prometheus/client_golang/api/prometheus/v1"
// 	"github.com/rancher/opni/pkg/alerting/drivers/backend"
// 	"github.com/rancher/opni/pkg/alerting/drivers/routing"
// 	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
// 	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
// 	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexadmin"
// 	"github.com/rancher/opni/plugins/metrics/pkg/apis/cortexops"
// 	"github.com/samber/lo"
// )

// // as opposed to an errGroup which stops on the first error,
// // this is a best effort to run all tasks and return all errors
// type independentErrGroup struct {
// 	errMu sync.Mutex
// 	errs  []error
// 	sync.WaitGroup
// }

// func (i *independentErrGroup) Add(tasks int) {
// 	i.WaitGroup.Add(tasks)
// }

// func (i *independentErrGroup) Done() {
// 	i.WaitGroup.Done()
// }

// func (i *independentErrGroup) Wait() {
// 	i.WaitGroup.Wait()
// }

// func (i *independentErrGroup) AddError(err error) {
// 	i.errMu.Lock()
// 	defer i.errMu.Unlock()
// 	i.errs = append(i.errs, err)
// }

// func (i *independentErrGroup) Error() error {
// 	if len(i.errs) == 0 {
// 		return nil
// 	}
// 	duped := map[string]struct{}{}
// 	resErr := []string{}
// 	for _, err := range i.errs {
// 		if _, ok := duped[err.Error()]; !ok {
// 			duped[err.Error()] = struct{}{}
// 			resErr = append(resErr, err.Error())
// 		}
// 	}
// 	sort.Strings(resErr)
// 	return fmt.Errorf(strings.Join(resErr, ","))
// }

// type alertStatusNotification lo.Tuple2[*alertingv1.AlertStatusResponse, error]
// type clusterMap map[string]*corev1.Cluster
// type statusInfo struct {
// 	mu           *sync.Mutex
// 	coreInfo     *coreInfo
// 	metricsInfo  *metricsInfo
// 	alertingInfo *alertingInfo
// }

// func (s *statusInfo) setMetricsInfo(i *metricsInfo) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	s.metricsInfo = i
// }

// func (s *statusInfo) setCoreInfo(i *coreInfo) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	s.coreInfo = i
// }

// func (s *statusInfo) setAlertingInfo(i *alertingInfo) {
// 	s.mu.Lock()
// 	defer s.mu.Unlock()
// 	s.alertingInfo = i
// }

// type coreInfo struct {
// 	clusterMap clusterMap
// }

// type metricsInfo struct {
// 	metricsBackendStatus *cortexops.InstallStatus
// 	cortexRules          *cortexadmin.RuleGroups
// }

// type alertingInfo struct {
// 	router          routing.OpniRouting
// 	alertGroup      []backend.GettableAlert
// 	loadedReceivers []string
// }

// func statusFromLoadedReceivers(
// 	cond *alertingv1.AlertCondition,
// 	alertInfo *alertingInfo,
// ) *alertingv1.AlertStatusResponse {
// 	matchers := alertInfo.router.HasLabels(cond.Id)
// 	requiredReceivers := alertInfo.router.HasReceivers(cond.Id)

// 	if len(requiredReceivers) == 0 ||
// 		cond.AttachedEndpoints == nil ||
// 		len(cond.AttachedEndpoints.Items) == 0 {
// 		return &alertingv1.AlertStatusResponse{
// 			State: alertingv1.AlertConditionState_Ok,
// 		}
// 	}
// 	matchingReceivers := lo.Intersect(requiredReceivers, alertInfo.loadedReceivers)
// 	if len(matchers) != 0 && len(matchingReceivers) != len(requiredReceivers) {
// 		return &alertingv1.AlertStatusResponse{
// 			State:  alertingv1.AlertConditionState_Pending,
// 			Reason: "alarm dependencies are updating",
// 		}
// 	}
// 	return &alertingv1.AlertStatusResponse{
// 		State: alertingv1.AlertConditionState_Ok,
// 	}
// }

// func statusFromAlertGroup(
// 	cond *alertingv1.AlertCondition,
// 	alertInfo *alertingInfo,
// ) *alertingv1.AlertStatusResponse {
// 	matchers := alertInfo.router.HasLabels(cond.Id)
// 	alertGroup := alertInfo.alertGroup
// 	if len(matchers) == 0 {
// 		return &alertingv1.AlertStatusResponse{
// 			State:  alertingv1.AlertConditionState_Pending,
// 			Reason: "alarm dependencies are updating",
// 		}
// 	}
// 	for _, alert := range alertGroup {
// 		// must match all matchers from the router spec to the alert's labels
// 		if !lo.EveryBy(matchers, func(m *labels.Matcher) bool {
// 			for labelName, label := range alert.Labels {
// 				if m.Name == labelName && m.Matches(label) {
// 					return true
// 				}
// 			}
// 			return false
// 		}) {
// 			continue // these are not the alerts you are looking for
// 		}
// 		switch *alert.Status.State {
// 		case models.AlertStatusStateSuppressed:
// 			return &alertingv1.AlertStatusResponse{
// 				State: alertingv1.AlertConditionState_Silenced,
// 			}
// 		case models.AlertStatusStateActive:
// 			return &alertingv1.AlertStatusResponse{
// 				State: alertingv1.AlertConditionState_Firing,
// 			}
// 		case models.AlertStatusStateUnprocessed:
// 			// in our case unprocessed means it has arrived for firing
// 			return &alertingv1.AlertStatusResponse{
// 				State: alertingv1.AlertConditionState_Firing,
// 			}
// 		default:
// 			return &alertingv1.AlertStatusResponse{
// 				State: alertingv1.AlertConditionState_Ok,
// 			}
// 		}
// 	}
// 	return &alertingv1.AlertStatusResponse{
// 		State: alertingv1.AlertConditionState_Ok,
// 	}
// }

// func evaluatePrometheusRuleHealth(ruleList *cortexadmin.RuleGroups, id string) *alertingv1.AlertStatusResponse {
// 	if ruleList == nil {
// 		return &alertingv1.AlertStatusResponse{
// 			State:  alertingv1.AlertConditionState_Pending,
// 			Reason: "cannot read rule state(s) from metrics backend",
// 		}
// 	}

// 	for _, group := range ruleList.GetGroups() {
// 		if strings.Contains(group.GetName(), id) {
// 			if len(group.GetRules()) == 0 {
// 				return &alertingv1.AlertStatusResponse{
// 					State:  alertingv1.AlertConditionState_Pending,
// 					Reason: "alarm metric dependencies are updating",
// 				}
// 			}
// 			healthList := lo.Map(group.GetRules(), func(rule *cortexadmin.Rule, _ int) string {
// 				return rule.GetHealth()
// 			})
// 			health := lo.Associate(healthList, func(health string) (string, struct{}) {
// 				return health, struct{}{}
// 			})
// 			if _, ok := health[promClient.RuleHealthBad]; ok {
// 				return &alertingv1.AlertStatusResponse{
// 					State:  alertingv1.AlertConditionState_Invalidated,
// 					Reason: "one ore more metric dependencies are unhealthy",
// 				}
// 			}
// 			if _, ok := health[promClient.RuleHealthUnknown]; ok {
// 				return &alertingv1.AlertStatusResponse{
// 					State:  alertingv1.AlertConditionState_Pending,
// 					Reason: "alarm metric dependencies are updating",
// 				}
// 			}
// 		}
// 	}
// 	return nil
// }
