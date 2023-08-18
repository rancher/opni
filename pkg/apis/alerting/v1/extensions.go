package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"slices"

	"github.com/google/uuid"
	"github.com/rancher/opni/pkg/alerting/message"
	alertingSync "github.com/rancher/opni/pkg/alerting/server/sync"
	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/samber/lo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	UpstreamClusterId        = "UPSTREAM_CLUSTER_ID"
	EndpointTagNotifications = "notifications"
)

// Note these properties have to conform to the AlertManager label naming convention
// https://prometheus.io/docs/alerting/latest/configuration/#labelname
const (
	// maps to a wellknown.Capability
	RoutingPropertyDatasource = "opni_datasource"
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
		message.NotificationPropertySeverity: n.GetProperties()[message.NotificationPropertySeverity],
		message.NotificationPropertyOpniUuid: n.GetProperties()[message.NotificationPropertyOpniUuid],
	}
	if v, ok := n.GetProperties()[message.NotificationPropertyDedupeKey]; ok {
		res[message.NotificationPropertyDedupeKey] = v
	} else {
		res[message.NotificationPropertyDedupeKey] = n.GetProperties()[message.NotificationPropertyOpniUuid]
	}

	if v, ok := n.GetProperties()[message.NotificationPropertyGroupKey]; ok {
		res[message.NotificationPropertyGroupKey] = v
	} else {
		// we have no knowledge of how to group messages by context
		res[message.NotificationPropertyGroupKey] = uuid.New().String()
	}
	return res
}

func (n *Notification) GetRoutingAnnotations() map[string]string {
	res := map[string]string{
		message.NotificationContentHeader:        n.Title,
		message.NotificationContentSummary:       n.Body,
		message.NotificationPropertyGoldenSignal: n.GetRoutingGoldenSignal(),
	}

	if v, ok := n.GetProperties()[message.NotificationPropertyClusterId]; ok {
		res[message.NotificationPropertyClusterId] = v
	}
	return lo.Assign(n.GetProperties(), res)
}

func (n *Notification) GetRoutingGoldenSignal() string {
	v, ok := n.GetProperties()[message.NotificationPropertyGoldenSignal]
	if ok {
		return v
	}
	return GoldenSignal_Custom.String()
}

func (n *Notification) Namespace() string {
	return message.NotificationPropertySeverity
}

func (a *AlertCondition) Hash() (string, error) {
	marshaler := proto.MarshalOptions{
		Deterministic: true,
	}
	md := a.GetMetadata()
	a.Metadata = nil
	data, err := marshaler.Marshal(a)
	a.Metadata = md
	if err != nil {
		return "", nil
	}
	hash := sha256.New()
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (a *AlertCondition) Visit(syncInfo alertingSync.SyncInfo) {
	if clusterMd, ok := syncInfo.Clusters[a.GetClusterId().Id]; ok {
		if a.Annotations == nil {
			a.Annotations = map[string]string{}
		}
		if clusterMd.Metadata != nil && clusterMd.Metadata.Labels != nil {
			if clusterName, ok := clusterMd.Metadata.Labels[corev1.NameLabel]; ok {
				a.Annotations[message.NotificationContentClusterName] = clusterName
			}
		}
	}
}

func (a *AlertCondition) Sanitize() {}

func (a *AlertCondition) GetRoutingLabels() map[string]string {
	res := map[string]string{
		message.NotificationPropertySeverity: a.GetSeverity().String(),
		message.NotificationPropertyOpniUuid: a.GetId(),
		a.Namespace():                        a.GetId(),
	}
	return res
}

func (a *AlertCondition) GetRoutingAnnotations() map[string]string {
	res := map[string]string{
		message.NotificationContentHeader:        a.header(),
		message.NotificationContentSummary:       a.body(),
		message.NotificationPropertyClusterId:    a.GetClusterId().GetId(),
		message.NotificationContentAlarmName:     a.GetName(),
		message.NotificationPropertyGoldenSignal: a.GetRoutingGoldenSignal(),
	}
	if IsMetricsCondition(a) {
		res[message.NotificationPropertyFingerprint] = fmt.Sprintf(
			`{{ "ALERTS_FOR_STATE{opni_uuid=\"%s\"} OR vector(0)" | query | first | value `,
			a.Id,
		) + `| printf "%.0f" }}`
	}
	return lo.Assign(a.GetAnnotations(), res)
}

func (a *AlertCondition) GetRoutingGoldenSignal() string {
	return a.GetGoldenSignal().String()
}

func (a *AlertCondition) DatasourceName() string {
	if IsInternalCondition(a) {
		return "alerting"
	}
	if IsMetricsCondition(a) {
		return "metrics"
	}
	return "unknown"
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
				if tEnd.Before(now.Add(-ttl)) { // check if we should prune it
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

func (l *ListAlertConditionRequest) FilterFunc() func(*AlertCondition, int) bool {
	return func(item *AlertCondition, _ int) bool {
		if len(l.Clusters) != 0 {
			if !slices.Contains(l.Clusters, item.GetClusterId().Id) {
				return false
			}
		}
		itemLabelMap := lo.Associate(item.Labels, func(label string) (string, struct{}) { return label, struct{}{} })
		if len(l.Labels) != 0 {
			for _, label := range l.Labels {
				if _, ok := itemLabelMap[label]; !ok {
					return false
				}
			}
		}
		if len(l.Severities) != 0 {
			if !slices.Contains(l.Severities, item.Severity) {
				return false
			}
		}
		if len(l.AlertTypes) != 0 {
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
