package config

/*
Implements format from
- github.com/prometheus/alertmanager/template
- github.com/prometheus/alertmanager/notify/webhook
*/

import (
	"time"

	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/samber/lo"
)

var _ message.Message = (*Alert)(nil)

// unmarshals messages received from webhooks
type WebhookMessage struct {
	Receiver string `json:"receiver"`
	Status   string `json:"status"`
	Alerts   Alerts `json:"alerts"`

	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`

	ExternalURL string `json:"externalURL"`

	// The protocol version.
	Version         string `json:"version"`
	GroupKey        string `json:"groupKey"`
	TruncatedAlerts uint64 `json:"truncatedAlerts"`
}

type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

func (a *Alert) GetHeader() (header string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	header, ok := lp.GetHeader()
	if !ok {
		return ap.GetSummary()
	}
	return header, ok
}

func (a *Alert) GetSummary() (summary string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	summary, ok := lp.GetSummary()
	if !ok {
		return ap.GetSummary()
	}
	return summary, ok
}

func (a *Alert) GetClusterName() (clusterName string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	clusterName, ok := lp.GetClusterName()
	if !ok {
		return ap.GetClusterName()
	}
	return clusterName, ok
}

func (a *Alert) GetAlarmName() (alarmName string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	alarmName, ok := lp.GetAlarmName()
	if !ok {
		return ap.GetAlarmName()
	}
	return alarmName, ok
}

// Alerts is a list of Alert objects.
type Alerts []Alert

// indicates whether this was a one time message, or a recurrent message like an alarm
func (a *Alert) IsPushNotification() (oneTime bool, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	isNotif, ok := lp.IsPushNotification()
	if !ok {
		return ap.IsPushNotification()
	}
	return isNotif, ok
}

func (a *Alert) GetUuid() (uuid string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	uuid, ok := lp.GetUuid()
	if !ok {
		return ap.GetUuid()
	}
	return uuid, ok
}

func (a *Alert) GetFingerprint() (fingerprint string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	fingerprint, ok := lp.GetFingerprint()
	if !ok {
		return ap.GetFingerprint()
	}
	return fingerprint, ok
}

func (a *Alert) GetDedupeKey() (dedupeKey string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	dedupe, ok := lp.GetDedupeKey()
	if !ok {
		return ap.GetDedupeKey()
	}
	return dedupe, ok
}

func (a *Alert) GetGroupKey() (groupKey string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	group, ok := lp.GetGroupKey()
	if !ok {
		return ap.GetGroupKey()
	}
	return group, ok
}

func (a *Alert) GetGoldenSignal() (goldenSignal string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	signal, ok := lp.GetGoldenSignal()
	if !ok {
		return ap.GetGoldenSignal()
	}
	return signal, ok
}

func (a *Alert) GetSeverity() (severity string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	severity, ok := lp.GetSeverity()
	if !ok {
		return ap.GetSeverity()
	}
	return severity, ok
}

func (a *Alert) GetClusterId() (clusterId string, found bool) {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	severity, ok := lp.GetClusterId()
	if !ok {
		return ap.GetClusterId()
	}
	return severity, ok
}

func (a *Alert) GetDetails() map[string]string {
	lp := message.Properties(a.Labels)
	ap := message.Properties(a.Annotations)
	return lo.Assign(lp.GetDetails(), ap.GetDetails())
}
