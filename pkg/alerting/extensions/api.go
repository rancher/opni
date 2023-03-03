package extensions

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/samber/lo"
)

// handleWebhook handles caching the incoming webhook messages
//
// We expect the request body will be in the form of AM webhook payload :
// https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
//
// Note :
//
//	   Webhooks are assumed to respond with 2xx response codes on a successful
//		  request and 5xx response codes are assumed to be recoverable.
//
// # Therefore, non-recoverable errors should have error codes 3XX and 4XX
//
// This HTTP handler needs to be able to handle a large throughput of requests
// so we make some performance optimizations
func (e *EmbeddedServer) handleWebhook(wr http.ResponseWriter, req *http.Request) {
	e.logger.Debug("handling webhook persistence request")
	wMsg, err := ParseIcomingWebhookMessage(req)
	if err != nil {
		wr.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	req.Body.Close()

	for _, alert := range wMsg.Alerts {
		msgMeta := parseLabelsToOpniMd(alert.Labels, alert.Annotations)
		if msgMeta.uuid == "" {
			// we assume a non-opni "indexed" source is pushing messages to us
			// we do not persist these as their format is not known
			e.logger.Debug("received message from non-opni source, ignoring")
			wr.WriteHeader(http.StatusOK)
			continue
		}

		if isAlarmMessage(alert.Annotations) {
			if err := e.cacheAlarm(msgMeta, alert); err != nil {
				wr.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		} else {
			if err := e.cacheNotification(msgMeta, alert); err != nil {
				wr.WriteHeader(http.StatusPreconditionFailed)
				return
			}
		}

	}
	wr.WriteHeader(http.StatusOK)
}

func (e *EmbeddedServer) handleListNotifications(wr http.ResponseWriter, req *http.Request) {
	var listRequest alertingv1.ListNotificationRequest
	if err := json.NewDecoder(req.Body).Decode(&listRequest); err != nil {
		e.logger.Error(err)
		wr.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := listRequest.Validate(); err != nil {
		e.logger.Error(err)
		wr.WriteHeader(http.StatusBadRequest)
		return
	}
	found := int32(0)
	res := alertingv1.ListMessageResponse{
		Items: make([]*alertingv1.MessageInstance, 0),
	}
	goldenSignals := lo.Associate(listRequest.GoldenSignalFilters, func(s alertingv1.GoldenSignal) (string, struct{}) {
		return s.String(), struct{}{}
	})

	// traverse by priority first
	partitionedKeys := e.notificationCache.PartitionedKeys()
	for _, severityKey := range listRequest.SeverityFilters {
		keys, ok := partitionedKeys[severityKey]
		if !ok {
			continue
		}
		for _, key := range keys {
			msg, _ := e.notificationCache.Get(severityKey, key)
			if _, ok := goldenSignals[lo.ValueOr(
				msg.Notification.Properties,
				alertingv1.NotificationPropertyGoldenSignal,
				alertingv1.OpniSeverity_Info.String(),
			)]; !ok {
				continue
			}
			res.Items = append(res.Items, msg)
			found++
			if found >= *listRequest.Limit {
				writeResponse(wr, &res)
				return
			}
		}
	}
	writeResponse(wr, &res)
}

func (e *EmbeddedServer) handleListAlarms(wr http.ResponseWriter, req *http.Request) {
	var listRequest alertingv1.ListAlarmMessageRequest
	if err := json.NewDecoder(req.Body).Decode(&listRequest); err != nil {
		wr.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := listRequest.Validate(); err != nil {
		wr.WriteHeader(http.StatusBadRequest)
		return
	}
	res := alertingv1.ListMessageResponse{
		Items: make([]*alertingv1.MessageInstance, 0),
	}

	reducedFingerprints := lo.Associate(listRequest.Fingerprints, func(item string) (string, struct{}) {
		return item, struct{}{}
	})

	n := len(listRequest.ConditionId)

	for severity, keys := range e.alarmCache.PartitionedKeys() {
		for _, key := range keys {
			if !strings.HasPrefix(
				key,
				listRequest.ConditionId,
			) {
				continue
			}

			if len(key) == n {
				// fallback : match purely based on timestamp
				msg, _ := e.alarmCache.Get(severity, key)
				received := msg.ReceivedAt.AsTime()
				lastUpdated := func() time.Time {
					if msg.LastUpdatedAt == nil {
						return time.Date(2070, 0, 0, 0, 0, 0, 0, time.Local)
					}
					return msg.LastUpdatedAt.AsTime()
				}()
				start, end := listRequest.Start.AsTime(), listRequest.End.AsTime()
				if between(start, end, received) ||
					between(start, end, lastUpdated) {
					res.Items = append(
						res.Items,
						msg,
					)
				}
			} else {
				// more deterministic matching using fingerprints
				for fingerprint := range reducedFingerprints {
					if strings.Contains(key, fingerprint) {
						msg, _ := e.alarmCache.Get(severity, key)
						res.Items = append(
							res.Items,
							msg)
					}
				}
			}
		}
	}
	writeResponse(wr, &res)
}

func isAlarmMessage(annotations map[string]string) bool {
	_, ok := annotations[shared.OpniAlarmNameAnnotation]
	return ok
}

func parseLabelsToOpniMd(labels, annotations map[string]string) messageMetadata {
	return messageMetadata{
		uuid:           lo.ValueOr(labels, alertingv1.NotificationPropertyOpniUuid, ""),
		groupDedupeKey: lo.ValueOr(labels, alertingv1.NotificationPropertyDedupeKey, ""),
		severity: lo.ValueOr(alertingv1.OpniSeverity_value,
			lo.ValueOr(labels, alertingv1.NotificationPropertySeverity, defaultSeverity),
			0,
		),
		fingerprint: lo.ValueOr(annotations, alertingv1.NotificationPropertyFingerprint, ""),
	}
}

// ParseIncomingWebhookMessage return non-pointer for performance reasons
func ParseIcomingWebhookMessage(req *http.Request) (config.WebhookMessage, error) {
	var wMsg config.WebhookMessage
	if err := json.NewDecoder(req.Body).Decode(&wMsg); err != nil {
		return config.WebhookMessage{}, err
	}
	return wMsg, nil
}

func writeResponse(wr http.ResponseWriter, res *alertingv1.ListMessageResponse) {
	wr.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(wr).Encode(res); err != nil {
		wr.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func between(start, end, ts time.Time) bool {
	return ts.After(start) && ts.Before(end)
}
