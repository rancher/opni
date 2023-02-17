package v1

import (
	"strings"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

const UpstreamClusterId = "UPSTREAM_CLUSTER_ID"
const EndpointTagDefault = "default"

func (r *RoutingRelationships) InvolvedConditionsForEndpoint(endpointId string) []string {
	res := []string{}
	for conditionId, endpointMap := range r.GetConditions() {
		for endpoint := range endpointMap.GetEndpoints() {
			if endpoint == endpointId {
				res = append(res, conditionId)
				break
			}
		}
	}
	return res
}

func IsInternalCondition(cond *AlertCondition) bool {
	if cond.GetAlertType().GetSystem() != nil ||
		cond.GetAlertType().GetDownstreamCapability() != nil ||
		cond.GetAlertType().GetMonitoringBackend() != nil {
		return true
	}
	return false
}

func IsMetricsCondition(cond *AlertCondition) bool {
	if cond.GetAlertType().GetPrometheusQuery() != nil ||
		cond.GetAlertType().GetKubeState() != nil ||
		cond.GetAlertType().GetCpu() != nil ||
		cond.GetAlertType().GetMemory() != nil ||
		cond.GetAlertType().GetFs() != nil {
		return true
	}
	return false
}

// stop-gap solution, until we move to the new versin of the API
func (a *AlertCondition) GetClusterId() *corev1.Reference {
	if a.GetAlertType().GetSystem() != nil {
		return a.GetAlertType().GetSystem().GetClusterId()
	}
	if a.GetAlertType().GetDownstreamCapability() != nil {
		return a.GetAlertType().GetDownstreamCapability().GetClusterId()
	}
	if a.GetAlertType().GetMonitoringBackend() != nil {
		return a.GetAlertType().GetMonitoringBackend().GetClusterId()
	}
	if a.GetAlertType().GetPrometheusQuery() != nil {
		return a.GetAlertType().GetPrometheusQuery().GetClusterId()
	}
	if a.GetAlertType().GetKubeState() != nil {
		return &corev1.Reference{Id: a.GetAlertType().GetKubeState().ClusterId}
	}
	if a.GetAlertType().GetCpu() != nil {
		return a.GetAlertType().GetCpu().GetClusterId()
	}
	if a.GetAlertType().GetMemory() != nil {
		return a.GetAlertType().GetMemory().GetClusterId()
	}
	if a.GetAlertType().GetFs() != nil {
		return a.GetAlertType().GetFs().GetClusterId()
	}
	return nil
}

func (a *AlertCondition) IsType(typVal AlertType) bool {
	switch typVal {
	case AlertType_System:
		return a.GetAlertType().GetSystem() != nil
	case AlertType_DownstreamCapability:
		return a.GetAlertType().GetDownstreamCapability() != nil
	case AlertType_MonitoringBackend:
		return a.GetAlertType().GetMonitoringBackend() != nil
	case AlertType_PrometheusQuery:
		return a.GetAlertType().GetPrometheusQuery() != nil
	case AlertType_KubeState:
		return a.GetAlertType().GetKubeState() != nil
	case AlertType_CpuSaturation:
		return a.GetAlertType().GetCpu() != nil
	case AlertType_MemorySaturation:
		return a.GetAlertType().GetMemory() != nil
	case AlertType_FsSaturation:
		return a.GetAlertType().GetFs() != nil
	default:
		return false
	}
}

// stop-gap solution until we move to the new version of the API
func (a *AlertCondition) SetClusterId(clusterId *corev1.Reference) error {
	if a.GetAlertType().GetSystem() != nil {
		a.GetAlertType().GetSystem().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetPrometheusQuery() != nil {
		a.GetAlertType().GetPrometheusQuery().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetKubeState() != nil {
		a.GetAlertType().GetKubeState().ClusterId = clusterId.Id
		return nil
	}
	if a.GetAlertType().GetCpu() != nil {
		a.GetAlertType().GetCpu().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetMemory() != nil {
		a.GetAlertType().GetMemory().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetFs() != nil {
		a.GetAlertType().GetFs().ClusterId = clusterId
		return nil
	}
	return shared.WithInternalServerErrorf("AlertCondition could not find its clusterId")
}

func (a *AlertCondition) GetTriggerAnnotations(conditionId string) map[string]string {
	res := map[string]string{}
	res[shared.BackendConditionSeverityLabel] = a.GetSeverity().String()
	res[shared.BackendConditionNameLabel] = a.GetName()
	res[shared.BackendConditionIdLabel] = conditionId
	res[shared.BackendConditionClusterIdLabel] = a.GetClusterId().Id
	if a.GetAlertType().GetSystem() != nil {
		res = lo.Assign(res, a.GetAlertType().GetSystem().GetTriggerAnnotations())
	}
	if a.GetAlertType().GetDownstreamCapability() != nil {
		res = lo.Assign(res, a.GetAlertType().GetDownstreamCapability().GetTriggerAnnotations())
	}
	if a.GetAlertType().GetMonitoringBackend() != nil {
		res = lo.Assign(res, a.GetAlertType().GetMonitoringBackend().GetTriggerAnnotations())
	}
	// prometheus query won't have specific template args
	return res
}

func (a *AlertConditionSystem) GetTriggerAnnotations() map[string]string {
	return map[string]string{
		"disconnectTimeout": a.GetTimeout().String(),
	}
}

func (a *AlertConditionDownstreamCapability) GetTriggerAnnotations() map[string]string {
	return map[string]string{
		"capabilitiesTracked": strings.Join(a.GetCapabilityState(), ","),
		"unhealthyThreshold":  a.GetFor().String(),
	}
}

func (a *AlertConditionMonitoringBackend) GetTriggerAnnotations() map[string]string {
	return map[string]string{
		"cortexComponents":   strings.Join(a.GetBackendComponents(), ","),
		"unhealthyThreshold": a.GetFor().String(),
	}
}

// noop
func (c *CachedState) RedactSecrets() {}

func (c *CachedState) IsEquivalent(other *CachedState) bool {
	return c.Healthy == other.Healthy && c.Firing == other.Firing
}

// if we can't read the last known state assume it is healthy
// and not firing, set last known state to now
func DefaultCachedState() *CachedState {
	return &CachedState{
		Healthy:   true,
		Firing:    false,
		Timestamp: timestamppb.Now(),
	}
}

// noop
func (i *IncidentIntervals) RedactSecrets() {}

func (i *IncidentIntervals) Prune(ttl time.Duration) {
	pruneIdx := 0
	now := time.Now()
	for _, interval := range i.GetItems() {
		// if is before the ttl, prune it
		tStart := interval.Start.AsTime()
		if tStart.Before(now.Add(-ttl)) {
			// if we know it ends before the known universe
			if interval.End != nil {
				tEnd := interval.End.AsTime()
				if tEnd.Before(now.Add(-ttl)) { //check if we should prune it
					pruneIdx++
				} else { // prune the start of the interval to before the ttl
					interval.Start = timestamppb.New(now.Add(-ttl).Add(time.Minute))
				}
			}
		} else {
			break // we can stop pruning
		}
	}
	i.Items = i.Items[pruneIdx:]
}

func NewIncidentIntervals() *IncidentIntervals {
	return &IncidentIntervals{
		Items: []*Interval{},
	}
}

func (r *RateLimitingConfig) Default() *RateLimitingConfig {
	r.ThrottlingDuration = durationpb.New(10 * time.Minute)
	r.RepeatInterval = durationpb.New(10 * time.Minute)
	r.InitialDelay = durationpb.New(10 * time.Second)
	return r
}

func (a *AlertCondition) Namespace() string {
	if a.GetAlertType().GetSystem() != nil {
		return "disconnect"
	}
	if a.GetAlertType().GetDownstreamCapability() != nil {
		return "capability"
	}
	if a.GetAlertType().GetCpu() != nil {
		return "cpu"
	}
	if a.GetAlertType().GetMemory() != nil {
		return "memory"
	}
	if a.GetAlertType().GetFs() != nil {
		return "fs"
	}
	if a.GetAlertType().GetKubeState() != nil {
		return "kube-state"
	}
	if a.GetAlertType().GetPrometheusQuery() != nil {
		return "promql"
	}
	if a.GetAlertType().GetMonitoringBackend() != nil {
		return "monitoring-backend"
	}
	return "default"
}

func (r *ListRoutingRelationshipsResponse) GetInvolvedConditions(endpointId string) *InvolvedConditions {
	involvedConditions := &InvolvedConditions{
		Items: []*corev1.Reference{},
	}
	for conditionId, endpointIds := range r.RoutingRelationships {
		if lo.Contains(
			lo.Map(endpointIds.Items, func(c *corev1.Reference, _ int) string { return c.Id }),
			endpointId) {
			involvedConditions.Items = append(involvedConditions.Items, &corev1.Reference{Id: conditionId})
		}
	}
	return involvedConditions
}
