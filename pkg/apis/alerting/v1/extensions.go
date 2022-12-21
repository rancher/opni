package v1

import (
	"reflect"
	"strings"
	"time"

	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

const upstreamClusterId = "UPSTREAM_CLUSTER_ID"

// EnumConditionToDatasource
//
// nil values indicate they are composition alerts
// we need this since each alert tree must only have one datasource atm
var EnumConditionToDatasource = make(map[AlertType]*string)

var EnumConditionToImplementation = make(map[AlertType]AlertTypeDetails)

func init() {
	//logging := shared.LoggingDatasource
	monitoring := shared.MonitoringDatasource
	system := shared.SystemDatasource
	EnumConditionToDatasource[AlertType_System] = &system
	EnumConditionToDatasource[AlertType_DownstreamCapability] = &system
	EnumConditionToDatasource[AlertType_KubeState] = &monitoring
	EnumConditionToDatasource[AlertType_CpuSaturation] = &monitoring
	EnumConditionToDatasource[AlertType_MemorySaturation] = &monitoring
	EnumConditionToDatasource[AlertType_FsSaturation] = &monitoring
	EnumConditionToDatasource[AlertType_Composition] = &monitoring
	EnumConditionToDatasource[AlertType_ControlFlow] = &monitoring
	EnumConditionToDatasource[AlertType_PrometheusQuery] = &monitoring
	EnumConditionToDatasource[AlertType_MonitoringBackend] = &system

	EnumConditionToImplementation[AlertType_System] = AlertTypeDetails{
		Type: &AlertTypeDetails_System{
			System: &AlertConditionSystem{},
		},
	}
	EnumConditionToImplementation[AlertType_KubeState] = AlertTypeDetails{
		Type: &AlertTypeDetails_KubeState{
			KubeState: &AlertConditionKubeState{},
		},
	}

	EnumConditionToImplementation[AlertType_CpuSaturation] = AlertTypeDetails{
		Type: &AlertTypeDetails_Cpu{
			Cpu: &AlertConditionCPUSaturation{},
		},
	}
	EnumConditionToImplementation[AlertType_MemorySaturation] = AlertTypeDetails{
		Type: &AlertTypeDetails_Memory{
			Memory: &AlertConditionMemorySaturation{},
		},
	}
	EnumConditionToImplementation[AlertType_FsSaturation] = AlertTypeDetails{
		Type: &AlertTypeDetails_Fs{
			Fs: &AlertConditionFilesystemSaturation{},
		},
	}
	EnumConditionToImplementation[AlertType_Composition] = AlertTypeDetails{
		Type: &AlertTypeDetails_Composition{
			Composition: &AlertConditionComposition{},
		},
	}
	EnumConditionToImplementation[AlertType_ControlFlow] = AlertTypeDetails{
		Type: &AlertTypeDetails_ControlFlow{
			ControlFlow: &AlertConditionControlFlow{},
		},
	}

	EnumConditionToImplementation[AlertType_PrometheusQuery] = AlertTypeDetails{
		Type: &AlertTypeDetails_PrometheusQuery{
			PrometheusQuery: &AlertConditionPrometheusQuery{},
		},
	}
	EnumConditionToImplementation[AlertType_DownstreamCapability] = AlertTypeDetails{
		Type: &AlertTypeDetails_DownstreamCapability{
			DownstreamCapability: &AlertConditionDownstreamCapability{},
		},
	}
	EnumConditionToImplementation[AlertType_MonitoringBackend] = AlertTypeDetails{
		Type: &AlertTypeDetails_MonitoringBackend{
			MonitoringBackend: &AlertConditionMonitoringBackend{},
		},
	}
}

func EnumHasImplementation(a AlertType) error {
	if _, ok := EnumConditionToImplementation[a]; !ok {
		return shared.AlertingErrNotImplemented
	}
	return nil
}

func DetailsHasImplementation(a *AlertTypeDetails) error {
	for _, v := range EnumConditionToImplementation {
		if reflect.TypeOf(v.GetType()) == reflect.TypeOf(a.GetType()) {
			return nil
		}
	}
	return shared.AlertingErrNotImplemented
}

type Templateable interface {
	ListTemplates() []string
}

var _ Templateable = &AlertTypeDetails{}

func (a *AlertTypeDetails) ListTemplates() []string {
	if a.GetSystem() != nil {
		return a.GetSystem().ListTemplates()
	}
	if a.GetKubeState() != nil {
		return a.GetKubeState().ListTemplates()
	}
	if a.GetComposition() != nil {
		return a.GetComposition().ListTemplates()
	}
	if a.GetControlFlow() != nil {
		return a.GetControlFlow().ListTemplates()
	}
	panic("No templates returned for alert type")
}
func (a *AlertConditionSystem) ListTemplates() []string {
	return []string{
		"agentId",
		"timeout",
	}
}

func (a *AlertConditionKubeState) ListTemplates() []string {
	return []string{}
}

func (a *AlertConditionComposition) ListTemplates() []string {
	return []string{}
}

func (a *AlertConditionControlFlow) ListTemplates() []string {
	return []string{}
}

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

func ShouldCreateRoutingNode(new, old *AttachedEndpoints) bool {
	// only create if we go from having no endpoints to having some
	if new == nil || len(new.Items) == 0 {
		return false
	} else if (old == nil || len(old.Items) == 0) && new != nil && len(new.Items) > 0 {
		return true
	}
	return false // should update pre-existing routing-node
}

func ShouldUpdateRoutingNode(new, old *AttachedEndpoints) bool {
	// only update if both are specified
	if new != nil && len(new.Items) > 0 && old != nil && len(old.Items) > 0 {
		return true
	}
	return false
}

func ShouldDeleteRoutingNode(new, old *AttachedEndpoints) bool {
	// only delete if we go from having endpoints to having none
	if (new == nil || len(new.Items) > 0) && old != nil && len(old.Items) > 0 {
		return true
	}
	return false
}

// stop-gap solution, until we move to the new versin of the API
func (a *AlertCondition) GetClusterId() *corev1.Reference {
	if a.GetAlertType().GetSystem() != nil {
		return a.GetAlertType().GetSystem().GetClusterId()
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

func (s *CachedState) IsEquivalent(other *CachedState) bool {
	return s.Healthy == other.Healthy && s.Firing == other.Firing
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
		tStart := interval.Start.AsTime()
		if tStart.Before(now.Add(-ttl)) {
			tEnd := interval.End.AsTime()
			if tEnd.Before(now.Add(-ttl)) {
				pruneIdx++
			} else {
				interval.Start = timestamppb.New(now.Add(-ttl).Add(time.Minute))
			}
		} else {
			break
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
