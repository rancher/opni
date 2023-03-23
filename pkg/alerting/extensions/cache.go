package extensions

import (
	"fmt"
	"strings"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	partitionProperty = "properties"
	partitionDetails  = "details"
)

func assignByPartition[K comparable, V any, T comparable](iteratee func(key K, value V) T, maps ...map[K]V) map[T]map[K]V {
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

// layered LFU cache
// L : Layer
// T : message contents
type messageCache[L comparable, T any] interface {
	Get(layer L, key string) (T, bool)
	Set(layer L, key string, msg config.Alert)
	// returns all keys ordered by (severity, heuristic(frequency, recency) )
	PartitionedKeys() map[L][]string
	Key(msg MessageMetadata) string
}

type lfuMessageCache map[alertingv1.OpniSeverity]*lru.TwoQueueCache[string, *alertingv1.MessageInstance]

var _ messageCache[alertingv1.OpniSeverity, *alertingv1.MessageInstance] = (*lfuMessageCache)(nil)

func NewLFUMessageCache(lub int) messageCache[alertingv1.OpniSeverity, *alertingv1.MessageInstance] {
	c := make(lfuMessageCache)
	i := float64(0)
	n := float64(len(alertingv1.OpniSeverity_value))

	sortedKeys := lo.Values(alertingv1.OpniSeverity_value)
	slices.SortFunc(sortedKeys, func(i, j int32) bool {
		return i < j
	})
	for _, severity := range sortedKeys {
		recentRatio := (n - i) * (1 / (1 + n))
		ghostRatio := ((i + 1) / (2 * n))

		notificationLayer, err := lru.New2QParams[string, *alertingv1.MessageInstance](lub, recentRatio, ghostRatio)
		if err != nil {
			panic(err)
		}
		c[alertingv1.OpniSeverity(severity)] = notificationLayer
		i++
	}
	return c
}

func (l lfuMessageCache) Get(severity alertingv1.OpniSeverity, key string) (*alertingv1.MessageInstance, bool) {
	v, ok := l[severity]
	if !ok {
		return nil, ok
	}

	msg, ok := v.Get(key)
	if !ok {
		return nil, ok
	}

	return msg, true
}

func (l lfuMessageCache) Set(severity alertingv1.OpniSeverity, key string, alert config.Alert) {
	v, ok := l[severity]
	if !ok {
		return
	}

	data, ok := v.Get(key)
	if ok {
		msg := data
		mapPartitions := assignByPartition(func(key, value string) string {
			if strings.HasPrefix(strings.ToLower(key), "opni") {
				return partitionProperty
			}
			return partitionDetails
		}, alert.Annotations, alert.Labels)

		msg.LastDetails = mapPartitions[partitionDetails]
		msg.LastUpdatedAt = timestamppb.Now()
		v.Add(key, msg)
		return
	}
	msg := toMessageInstance(alert)
	v.Add(key, msg)
}

func toMessageInstance(alert config.Alert) *alertingv1.MessageInstance {
	msg := &alertingv1.MessageInstance{
		ReceivedAt: timestamppb.Now(),
		Notification: &alertingv1.Notification{
			Title: truncateMessageContent(
				lo.ValueOr(alert.Annotations, shared.OpniHeaderAnnotations, missingTitle),
			),
			Body: truncateMessageContent(
				lo.ValueOr(alert.Annotations, shared.OpniBodyAnnotations, missingBody),
			),
			Properties: map[string]string{
				alertingv1.NotificationPropertySeverity: lo.ValueOr(
					alert.Labels,
					alertingv1.NotificationPropertySeverity,
					alertingv1.OpniSeverity_Info.String(),
				),
				alertingv1.NotificationPropertyGoldenSignal: lo.ValueOr(
					alert.Annotations,
					alertingv1.NotificationPropertyGoldenSignal,
					alertingv1.GoldenSignal_Custom.String(),
				),
			},
		},
		StartDetails: map[string]string{},
		LastDetails:  map[string]string{},
	}
	mapPartitions := assignByPartition(func(key, value string) string {
		if strings.HasPrefix(key, "opni") || strings.HasPrefix(key, "Opni") {
			return partitionProperty
		}
		return partitionDetails
	}, alert.Annotations, alert.Labels)

	msg.StartDetails = lo.ValueOr(mapPartitions, partitionDetails, map[string]string{})
	msg.Notification.Properties = lo.Assign(msg.Notification.Properties, lo.ValueOr(mapPartitions, partitionProperty, map[string]string{}))
	return msg
}

func (l lfuMessageCache) Key(msgMeta MessageMetadata) string {
	if msgMeta.IsAlarm {
		if msgMeta.Fingerprint != "" {
			return fmt.Sprintf("%s-%s", msgMeta.Uuid, msgMeta.Fingerprint)
		}
		return msgMeta.Uuid
	}
	if msgMeta.GroupDedupeKey != "" {
		return msgMeta.GroupDedupeKey
	}
	return msgMeta.Uuid
}

func (l lfuMessageCache) PartitionedKeys() map[alertingv1.OpniSeverity][]string {
	traverseMessageKeys := lo.Values(alertingv1.OpniSeverity_value)
	// traverse by descending severity
	slices.SortFunc(traverseMessageKeys, func(a, b int32) bool {
		return a > b
	})
	returnKeys := map[alertingv1.OpniSeverity][]string{}
	for _, severityKey := range traverseMessageKeys {
		kk := l[alertingv1.OpniSeverity(severityKey)].Keys()
		s := make([]string, len(kk))
		for i, v := range kk {
			s[i] = fmt.Sprint(v)
		}
		returnKeys[alertingv1.OpniSeverity(severityKey)] = s
	}
	return returnKeys
}

func (e *EmbeddedServer) cacheAlarm(msgMeta MessageMetadata, alert config.Alert) error {
	e.alarmCache.Set(alertingv1.OpniSeverity(msgMeta.Severity), e.alarmCache.Key(msgMeta), alert)
	return nil
}

func (e *EmbeddedServer) cacheNotification(msgMeta MessageMetadata, alert config.Alert) error {
	e.notificationCache.Set(alertingv1.OpniSeverity(msgMeta.Severity), e.notificationCache.Key(msgMeta), alert)
	return nil
}
