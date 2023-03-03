package v1

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/capabilities/wellknown"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

const UpstreamClusterId = "UPSTREAM_CLUSTER_ID"
const EndpointTagNotifications = "notifications"

// Note these properties have to conform to the AlertManager label naming convention
// https://prometheus.io/docs/alerting/latest/configuration/#labelname
const (
	// maps to a wellknown.Capability
	RoutingPropertyDatasource = "opni_datasource"
)

var (
	OpniSubRoutingTreeMatcher *labels.Matcher = &labels.Matcher{
		Type:  labels.MatchEqual,
		Name:  RoutingPropertyDatasource,
		Value: "",
	}

	OpniMetricsSubRoutingTreeMatcher *labels.Matcher = &labels.Matcher{
		Type:  labels.MatchEqual,
		Name:  RoutingPropertyDatasource,
		Value: wellknown.CapabilityMetrics,
	}

	OpniSeverityTreeMatcher *labels.Matcher = &labels.Matcher{
		Type:  labels.MatchNotEqual,
		Name:  NotificationPropertySeverity,
		Value: "",
	}
)

// Note these properties have to conform to the AlertManager label naming convention
// https://prometheus.io/docs/alerting/latest/configuration/#labelname
const (
	// Property that specifies the unique identifier for the notification
	// Corresponds to condition id for `AlertCondition` and an opaque identifier for
	// an each ephemeral `Notification` instance
	NotificationPropertyOpniUuid = "opni_uuid"
	// Any messages already in the notification queue with the same dedupe key will not be processed
	// immediately, but will be deduplicated.
	NotificationPropertyDedupeKey = "opni_dedupe_key"
	// Any messages with the same group key will be sent together
	NotificationPropertyGroupKey = "opni_group_key"
	// Opaque identifier for the cluster that generated the notification
	NotificationPropertyClusterId = "opni_clusterId"
	// Property that specifies how to classify the notification according to golden signal
	NotificationPropertyGoldenSignal = "opni_goldenSignal"
	// Property that specifies the severity of the notification. Severity impacts how quickly the
	// notification is dispatched & repeated, as well as how long to persist it.
	NotificationPropertySeverity = "opni_severity"
	// Property that is used to correlate messages to particular incidents
	NotificationPropertyFingerprint = "opni_fingerprint"
)

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

func (n *Notification) GetRoutingLabels() map[string]string {
	res := map[string]string{
		NotificationPropertySeverity: n.GetProperties()[NotificationPropertySeverity],
		NotificationPropertyOpniUuid: n.GetProperties()[NotificationPropertyOpniUuid],
	}
	if v, ok := n.GetProperties()[NotificationPropertyDedupeKey]; ok {
		res[NotificationPropertyDedupeKey] = v
	} else {
		res[NotificationPropertyDedupeKey] = n.GetProperties()[NotificationPropertyOpniUuid]
	}

	if v, ok := n.GetProperties()[NotificationPropertyGroupKey]; ok {
		res[NotificationPropertyGroupKey] = v
	} else {
		// we have no knowledge of how to group messages by context
		res[NotificationPropertyGroupKey] = uuid.New().String()
	}
	return res
}

func (n *Notification) GetRoutingAnnotations() map[string]string {
	res := map[string]string{
		shared.OpniHeaderAnnotations:      n.Title,
		shared.OpniBodyAnnotations:        n.Body,
		shared.OpniGoldenSignalAnnotation: n.GetRoutingGoldenSignal(),
	}

	if v, ok := n.GetProperties()[NotificationPropertyClusterId]; ok {
		res[shared.OpniClusterAnnotation] = v
	}
	return res
}

func (n *Notification) GetRoutingGoldenSignal() string {
	v, ok := n.GetProperties()[NotificationPropertyGoldenSignal]
	if ok {
		return v
	}
	return GoldenSignal_Custom.String()
}

func (n *Notification) Namespace() string {
	return NotificationPropertySeverity
}

func (a *AlertCondition) GetRoutingLabels() map[string]string {
	res := map[string]string{
		NotificationPropertySeverity: a.GetSeverity().String(),
		NotificationPropertyOpniUuid: a.GetId(),
		a.Namespace():                a.GetId(),
	}
	return res
}

func (a *AlertCondition) GetRoutingAnnotations() map[string]string {
	res := map[string]string{
		shared.OpniHeaderAnnotations:      a.header(),
		shared.OpniBodyAnnotations:        a.body(),
		shared.OpniClusterAnnotation:      a.GetClusterId().GetId(),
		shared.OpniAlarmNameAnnotation:    a.GetName(),
		shared.OpniGoldenSignalAnnotation: a.GetRoutingGoldenSignal(),
	}
	if IsMetricsCondition(a) {
		res[NotificationPropertyFingerprint] = fmt.Sprintf(
			"{{ \"ALERTS_FOR_STATE{opni_uuid=\"%s\"}\" | query | first | value }}",
			a.Id,
		)
	}
	return res
}

func (a *AlertCondition) GetRoutingGoldenSignal() string {
	return a.GetGoldenSignal().String()
}

func (a *AlertCondition) header() string {
	// check custom user-set title
	if ae := a.GetAttachedEndpoints(); ae != nil {
		if ae.Details != nil {
			if ae.Details.Title != "" {
				return ae.Details.Title
			}
		}
	}
	return a.GetName()
}

func (a *AlertCondition) body() string {
	// check custom user-set body
	if ae := a.GetAttachedEndpoints(); ae != nil {
		if ae.Details != nil {
			if ae.Details.Title != "" {
				return ae.Details.Body
			}
		}
	}

	// otherwise check description
	if desc := a.GetDescription(); desc != "" {
		return desc
	}

	// fallback on default descriptions based on alert type
	if fallback := a.GetAlertType().body(); fallback != "" {
		return fallback
	}

	return "Sorry, no alarm body available for this alert type"
}

func (a *AlertTypeDetails) body() string {
	if a.GetSystem() != nil {
		return "Agent disconnect"
	}
	if a.GetDownstreamCapability() != nil {
		return "Downstream cluster capability"
	}
	if a.GetMonitoringBackend() != nil {
		return "Monitoring backend"
	}
	if a.GetPrometheusQuery() != nil {
		return "Prometheus query"
	}
	if a.GetKubeState() != nil {
		return "Kube state"
	}
	if a.GetCpu() != nil {
		return "CPU"
	}
	if a.GetMemory() != nil {
		return "Memory"
	}
	if a.GetFs() != nil {
		return "Filesystem"
	}
	return ""
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

func (i *IncidentIntervals) Smooth(_ time.Duration) {
	// TODO : need to smooth based on evaluation interval
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
		return "opni_disconnect"
	}
	if a.GetAlertType().GetDownstreamCapability() != nil {
		return "opni_capability"
	}
	if a.GetAlertType().GetCpu() != nil {
		return "opni_cpu"
	}
	if a.GetAlertType().GetMemory() != nil {
		return "opni_memory"
	}
	if a.GetAlertType().GetFs() != nil {
		return "opni_fs"
	}
	if a.GetAlertType().GetKubeState() != nil {
		return "opni_kube_state"
	}
	if a.GetAlertType().GetPrometheusQuery() != nil {
		return "opni_promql"
	}
	if a.GetAlertType().GetMonitoringBackend() != nil {
		return "opni_monitoring_backend"
	}
	return "opni_default"
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
func (l *ListAlertConditionRequest) FilterFunc() func(*AlertCondition, int) bool {
	return func(item *AlertCondition, _ int) bool {
		if len(l.Clusters) != 0 {
			if !slices.Contains(l.Clusters, item.GetClusterId().Id) {
				return false
			}
		}
		if len(l.Labels) != 0 {
			if len(lo.Intersect(l.Labels, item.Labels)) != len(l.Labels) {
				return false
			}
		}
		if len(l.Severities) != 0 {
			if !slices.Contains(l.Severities, item.Severity) {
				return false
			}
		}
		if len(l.Severities) != 0 {
			matches := false
			for _, typ := range l.GetAlertTypes() {
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
	}
}
