package message

import "strings"

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

const (
	// Contains the contain of the alert title
	NotificationContentHeader = "OpniHeader"
	// Contains the body of the alert message
	NotificationContentSummary = "OpniSummary"
	// Contains the friendly cluster name of the notification
	NotificationContentClusterName = "OpniClusterName"
	// Contains the friendly alarm name of the notification
	NotificationContentAlarmName = "OpniAlarmName"
)

const (
	NotificationPartitionByProperty = "properties"
	NotificationPartitionByDetails  = "details"
)

// var _ Message = (*config.Alert)(nil)

// Identifies important information in message contents
type Message interface {
	// Routing properties

	// indicates whether this was a one time message, or a recurrent message like an alarm
	IsPushNotification() (oneTime, found bool)
	GetUuid() (uuid string, found bool)
	GetFingerprint() (fingerprint string, found bool)
	GetDedupeKey() (dedupeKey string, found bool)
	GetGroupKey() (groupKey string, found bool)
	GetGoldenSignal() (goldenSignal string, found bool)
	GetSeverity() (severity string, found bool)
	GetClusterId() (clusterId string, found bool)

	// Alert Contents

	GetHeader() (header string, found bool)
	GetSummary() (summary string, found bool)
	GetClusterName() (clusterName string, found bool)
	GetAlarmName() (alarmName string, found bool)

	// Details
	GetDetails() map[string]string
}

type Properties map[string]string

// indicates whether this was a one time message, or a recurrent message like an alarm
func (p Properties) IsPushNotification() (oneTime, found bool) {
	// TODO: this is weird
	_, ok := p[NotificationContentAlarmName]
	return !ok, false
}

func (p Properties) GetUuid() (string, bool) {
	prop, ok := p[NotificationPropertyOpniUuid]
	return prop, ok
}

func (p Properties) GetFingerprint() (string, bool) {
	prop, ok := p[NotificationPropertyFingerprint]
	return prop, ok
}

func (p Properties) GetDedupeKey() (string, bool) {
	prop, ok := p[NotificationPropertyDedupeKey]
	return prop, ok
}

func (p Properties) GetGroupKey() (string, bool) {
	prop, ok := p[NotificationPropertyGroupKey]
	return prop, ok
}

func (p Properties) GetGoldenSignal() (string, bool) {
	prop, ok := p[NotificationPropertyGoldenSignal]
	return prop, ok
}

func (p Properties) GetSeverity() (string, bool) {
	prop, ok := p[NotificationPropertySeverity]
	return prop, ok
}

func (p Properties) GetClusterId() (string, bool) {
	prop, ok := p[NotificationPropertyClusterId]
	return prop, ok
}

func (p Properties) GetHeader() (string, bool) {
	prop, ok := p[NotificationContentHeader]
	return prop, ok
}

func (p Properties) GetSummary() (string, bool) {
	prop, ok := p[NotificationContentSummary]
	return prop, ok
}

func (p Properties) GetClusterName() (string, bool) {
	prop, ok := p[NotificationContentClusterName]
	return prop, ok
}

func (p Properties) GetAlarmName() (string, bool) {
	prop, ok := p[NotificationContentAlarmName]
	return prop, ok
}

func (p Properties) GetDetails() map[string]string {
	partition := AssignByPartition(func(key, value string) string {
		if strings.HasPrefix(strings.ToLower(key), "opni") {
			return NotificationPartitionByProperty
		}
		return NotificationPartitionByDetails
	}, p)
	return partition[NotificationPartitionByDetails]
}

func AssignByPartition[K comparable, V any, T comparable](iteratee func(key K, value V) T, maps ...map[K]V) map[T]map[K]V {
	out := map[T]map[K]V{}

	for _, m := range maps {
		for k, v := range m {
			partition := iteratee(k, v)
			if _, ok := out[partition]; !ok {
				out[partition] = map[K]V{}
			}
			out[partition][k] = v
		}
	}

	return out
}
