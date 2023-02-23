package extensions

/*
Contains the AlertManager Opni embedded server implementation.
The embedded service must be run within the same process as each
deploymed node in the AlertManager cluster.
*/

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/rancher/opni/pkg/alerting/drivers/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	// add profiles
	_ "net/http/pprof"
)

const (
	missingTitle = "missing title"
	missingBody  = "missing body"
)

var defaultSeverity = alertingv1.OpniSeverity_Info.String()

type messageMetadata struct {
	uuid           string
	groupDedupeKey string
	severity       int32
}

// Embedded Server handles all incoming webhook requests from the AlertManager
//
// It needs to be simple and optimized for high throughput so we make some
// native performance optimizations:
//   - Maintain *lru.TwoQueueCache for each pair of  (opni message type, opni severity)
//   - Do not define map[string]*Cache for each cache pair to avoid additional lock contention
//   - Maintain a least upper bound` on the size of the combined caches
//
// We do make the tradeoff of maintaining `TwoQueueCache` instead of regular `LRU` cache to also
// account for frequency.
type EmbeddedServer struct {
	logger *zap.SugaredLogger
	// maxSize of the combined caches
	lub               int
	notificationMu    *sync.RWMutex
	alarmMu           *sync.RWMutex
	notificationCache map[int32]*lru.TwoQueueCache
	alarmCache        map[int32]*lru.TwoQueueCache
}

func NewEmbeddedServer(
	lg *zap.SugaredLogger,
	lub int,
) *EmbeddedServer {
	c1 := make(map[int32]*lru.TwoQueueCache)
	c2 := make(map[int32]*lru.TwoQueueCache)
	for _, severity := range alertingv1.OpniSeverity_value {
		// TODO : param heuristic
		notificationLayer, err := lru.New2QParams(lub, 0.8, 0.2)
		if err != nil {
			panic(err)
		}
		alarmLayer, err := lru.New2QParams(lub, 0.8, 0.2)
		if err != nil {
			panic(err)
		}
		c1[severity] = notificationLayer
		c2[severity] = alarmLayer
	}
	return &EmbeddedServer{
		logger:            lg,
		lub:               lub,
		notificationMu:    &sync.RWMutex{},
		alarmMu:           &sync.RWMutex{},
		notificationCache: c1,
		alarmCache:        c2,
	}
}

func isAlarmMessage(annotations map[string]string) bool {
	_, ok := annotations[shared.OpniAlarmNameAnnotation]
	return ok
}

func parseLabels(labels map[string]string) messageMetadata {
	return messageMetadata{
		uuid:           lo.ValueOr(labels, alertingv1.NotificationPropertyOpniUuid, ""),
		groupDedupeKey: lo.ValueOr(labels, alertingv1.NotificationPropertyDedupeKey, ""),
		severity: lo.ValueOr(alertingv1.OpniSeverity_value,
			lo.ValueOr(labels, alertingv1.NotificationPropertySeverity, defaultSeverity),
			0,
		),
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

// cacheKey
// conditionId is just an opaque id attached to a message.
// In the case of alarms it uniquely identifies an alarm instance, while for messages
// it uniquely identifies a message instance -- which means that the same message(s)
// that are not deduped properly will have different conditionIds
func (e *EmbeddedServer) cacheKey(groupKey string, conditionId string) string {
	if groupKey == "" {
		return conditionId
	}
	return groupKey
}

// HandleWebhook
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
func (e *EmbeddedServer) HandleWebhook(wr http.ResponseWriter, req *http.Request) {
	e.logger.Debug("handling webhook persistence request")
	wMsg, err := ParseIcomingWebhookMessage(req)
	if err != nil {
		wr.WriteHeader(http.StatusPreconditionFailed)
		return
	}
	req.Body.Close()

	for _, alert := range wMsg.Alerts {
		msgMeta := parseLabels(alert.Labels)
		if msgMeta.uuid == "" {
			// we assume a non-opni "indexed" source is pushing messages to us
			// we do not persist these as their format is not known
			e.logger.Debug("received message from non-opni source, ignoring")
			wr.WriteHeader(http.StatusOK)
			continue
		}

		if isAlarmMessage(alert.Annotations) {
			cache, ok := e.alarmCache[msgMeta.severity]
			if !ok {
				wr.WriteHeader(http.StatusPreconditionFailed)
				return
			}
			cache.Add(e.cacheKey(msgMeta.groupDedupeKey, msgMeta.uuid), &alertingv1.Notification{
				Title: lo.ValueOr(alert.Annotations, shared.OpniHeaderAnnotations, missingTitle),
				Body:  lo.ValueOr(alert.Annotations, shared.OpniBodyAnnotations, missingBody),
				Properties: map[string]string{
					alertingv1.NotificationPropertySeverity:     lo.ToPtr(alertingv1.OpniSeverity(msgMeta.severity)).String(),
					alertingv1.NotificationPropertyGoldenSignal: lo.ValueOr(alert.Annotations, shared.OpniGoldenSignalAnnotation, alertingv1.OpniSeverity_Info.String()),
				},
			})
			wr.WriteHeader(http.StatusOK)
			return
		}

		cache, ok := e.notificationCache[msgMeta.severity]
		if !ok {
			wr.WriteHeader(http.StatusPreconditionFailed)
			return
		}
		cache.Add(e.cacheKey(msgMeta.groupDedupeKey, msgMeta.uuid), &alertingv1.Notification{
			Title: lo.ValueOr(alert.Annotations, shared.OpniHeaderAnnotations, missingTitle),
			Body:  lo.ValueOr(alert.Annotations, shared.OpniBodyAnnotations, missingBody),
			Properties: map[string]string{
				alertingv1.NotificationPropertySeverity:     lo.ToPtr(alertingv1.OpniSeverity(msgMeta.severity)).String(),
				alertingv1.NotificationPropertyGoldenSignal: lo.ValueOr(alert.Annotations, shared.OpniGoldenSignalAnnotation, alertingv1.GoldenSignal_Custom.String()),
			},
		})
	}

	wr.WriteHeader(http.StatusOK)
}

func writeResponse(wr http.ResponseWriter, res *alertingv1.ListMessageResponse) {
	wr.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(wr).Encode(res); err != nil {
		wr.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (e *EmbeddedServer) HandleListNotifications(wr http.ResponseWriter, req *http.Request) {
	var listRequest alertingv1.ListMessageRequest
	if err := json.NewDecoder(req.Body).Decode(&listRequest); err != nil {
		wr.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := listRequest.Validate(); err != nil {
		wr.WriteHeader(http.StatusBadRequest)
		return
	}

	foundAlarms := int32(0)
	res := alertingv1.ListMessageResponse{
		Items: make([]*alertingv1.Notification, 0),
	}
	goldenSignals := lo.Associate(listRequest.GoldenSignalFilters, func(s alertingv1.GoldenSignal) (string, struct{}) {
		return s.String(), struct{}{}
	})

	traverseAlarmKeys := lo.Map(listRequest.SeverityFilters, func(s alertingv1.OpniSeverity, _ int) int32 {
		return int32(s)
	})
	// traverse by descending severity
	slices.SortFunc(traverseAlarmKeys, func(a, b int32) bool {
		return a > b
	})

	// traverse by priority first
	for _, severityKey := range traverseAlarmKeys {
		for _, key := range e.alarmCache[severityKey].Keys() {
			msg, _ := e.alarmCache[severityKey].Get(key)
			notif := msg.(*alertingv1.Notification)
			properties := notif.Properties
			if properties == nil {
				properties = map[string]string{}
			}
			if _, ok := goldenSignals[lo.ValueOr(
				properties,
				alertingv1.NotificationPropertyGoldenSignal,
				alertingv1.OpniSeverity_Info.String(),
			)]; !ok {
				continue
			}
			res.Items = append(res.Items, msg.(*alertingv1.Notification))
			foundAlarms++
			if foundAlarms >= *listRequest.Limit {
				writeResponse(wr, &res)
				return
			}
		}

		for _, key := range e.notificationCache[severityKey].Keys() {
			msg, _ := e.notificationCache[severityKey].Get(key)
			notif := msg.(*alertingv1.Notification)
			properties := notif.Properties
			if properties == nil {
				properties = map[string]string{}
			}
			if _, ok := goldenSignals[lo.ValueOr(
				properties,
				alertingv1.NotificationPropertyGoldenSignal,
				alertingv1.OpniSeverity_Info.String(),
			)]; !ok {
				continue
			}
			res.Items = append(res.Items, msg.(*alertingv1.Notification))
			foundAlarms++
			if foundAlarms >= *listRequest.Limit {
				writeResponse(wr, &res)
				return
			}
		}
	}
	writeResponse(wr, &res)
}

func StartOpniEmbeddedServer(ctx context.Context, opniAddr string) *http.Server {
	lg := logger.NewPluginLogger().Named("opni.alerting")
	es := NewEmbeddedServer(lg, 125)
	mux := http.NewServeMux()

	mux.HandleFunc("/list", es.HandleListNotifications)
	// request body will be in the form of AM webhook payload :
	// https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
	//
	// Note :
	//    Webhooks are assumed to respond with 2xx response codes on a successful
	//	  request and 5xx response codes are assumed to be recoverable.
	// therefore, non-recoverable errors should have error codes 3XX and 4XX
	mux.HandleFunc(shared.AlertingDefaultHookName, es.HandleWebhook)

	hookServer := &http.Server{
		// explicitly set this to 0.0.0.0 for test environment
		Addr:    opniAddr,
		Handler: mux,
	}
	go func() {
		err := hookServer.ListenAndServe()
		if !errors.Is(err, http.ErrServerClosed) {
			panic(err)
		}
	}()
	go func() {
		<-ctx.Done()
		if err := hookServer.Close(); err != nil {
			panic(err)
		}
	}()
	return hookServer
}
