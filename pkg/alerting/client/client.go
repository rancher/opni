package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-openapi/strfmt"
	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/rancher/opni/pkg/alerting/fingerprint"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	"google.golang.org/protobuf/encoding/protojson"
)

type AlertingPeer struct {
	ApiAddress      string
	EmbeddedAddress string
}

type AlertObject struct {
	Id          string
	Labels      map[string]string
	Annotations map[string]string
}

func (a *AlertObject) Validate() error {
	if a.Id == "" {
		return fmt.Errorf("alert object must have an id")
	}
	if a.Labels == nil {
		return fmt.Errorf("alert object must have labels")
	}
	if a.Annotations == nil {
		return fmt.Errorf("alert object must have annotations")
	}
	return nil
}

// A typical HA setup of prometheus operator assumes that alert sources
// (in particular Prometheus), triggers api calls to all known peers
// and alert status & deduplication will be handled by its memberlisted nflog.
type client struct {
	*http.Client

	peerMu     sync.RWMutex
	knownPeers []AlertingPeer

	// default alertmanager address
	alertmanagerAddress string
	// default querier address
	querierAddress string
}

var _ Client = (*client)(nil)

func NewClient(
	httpClient *http.Client,
	alertmanagerAddress string,
	querierAddress string,
) *client {
	c := &client{
		Client:              http.DefaultClient,
		alertmanagerAddress: alertmanagerAddress,
		querierAddress:      querierAddress,
		knownPeers:          []AlertingPeer{},
		peerMu:              sync.RWMutex{},
	}
	if httpClient != nil {
		c.Client = httpClient
	}
	return c
}

// A typical HA setup of prometheus operator assumes that alert sources
// (in particular Prometheus), triggers api calls to all known peers
// and alert status & deduplication will be handled by its memberlisted nflog.
//
// In our alerting client abstraction
type Client interface {
	MemberlistClient
	ControlClient
	StatusClient
	ConfigClient
	AlertClient
	QueryClient
	SilenceClient
	ProxyClient
}

type ProxyClient interface {
	ProxyRequest(ctx context.Context, req *http.Request) (*http.Response, error)
}

type ControlClient interface {
	Reload(ctx context.Context) error
}

type MemberlistClient interface {
	MemberPeers() []AlertingPeer
	// Service Discovery for alerting api clients is delegated
	// to an external mechanism, such as cluster drivers
	SetKnownPeers(peers []AlertingPeer)
}

type StatusClient interface {
	Status(ctx context.Context) ([]alertmanagerv2.AlertmanagerStatus, error)
	Ready(ctx context.Context) error
}

type ConfigClient interface {
	GetReceiver(ctx context.Context, id string) (alertmanagerv2.Receiver, error)
	ListReceivers(ctx context.Context) ([]alertmanagerv2.Receiver, error)
}

type AlertClient interface {
	ListAlerts(ctx context.Context) (alertmanagerv2.AlertGroups, error)
	GetAlert(ctx context.Context, labels []*labels.Matcher) (alertmanagerv2.GettableAlerts, error)
	PostAlarm(ctx context.Context, obj AlertObject) error
	PostNotification(ctx context.Context, obj AlertObject) error
	ResolveAlert(ctx context.Context, obj AlertObject) error
}

type QueryClient interface {
	ListAlarmMessages(context.Context, *alertingv1.ListAlarmMessageRequest) (*alertingv1.ListMessageResponse, error)
	ListNotificationMessages(context.Context, *alertingv1.ListNotificationRequest) (*alertingv1.ListMessageResponse, error)
}

type SilenceClient interface {
	ListSilences(ctx context.Context) (alertmanagerv2.GettableSilences, error)
	PostSilence(
		ctx context.Context,
		alarmId string,
		dur time.Duration,
		incomingSilenceId *string,
	) (silenceId string, err error)
	DeleteSilence(
		ctx context.Context,
		silenceId string,
	) error
}

func (c *client) Reload(ctx context.Context) error {
	addrs := c.MemberPeers()
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/-/reload", addr.ApiAddress), nil)
		if err != nil {
			return err
		}
		resp, err := c.Client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}
	return nil
}

func (c *client) ProxyRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	addrs := c.MemberPeers()
	incomingPath := req.URL.Path
	newPath := strings.TrimPrefix("/plugin_alerting/alertmanager", incomingPath)

	for _, addr := range addrs {
		newReq, err := http.NewRequestWithContext(ctx, req.Method, path.Join(addr.ApiAddress, newPath), req.Body)
		if err != nil {
			return nil, err
		}
		resp, err := c.Client.Do(newReq)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
	return nil, fmt.Errorf("no available endpoints for proxying")
}

func (c *client) MemberPeers() []AlertingPeer {
	c.peerMu.RLock()
	defer c.peerMu.RUnlock()
	if len(c.knownPeers) == 0 {
		return []AlertingPeer{
			{
				ApiAddress:      c.alertmanagerAddress,
				EmbeddedAddress: c.querierAddress,
			},
		}
	}
	return c.knownPeers
}

func (c *client) SetKnownPeers(peers []AlertingPeer) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()
	c.knownPeers = peers
}

func (c *client) Status(ctx context.Context) ([]alertmanagerv2.AlertmanagerStatus, error) {
	addrs := c.MemberPeers()
	res := []alertmanagerv2.AlertmanagerStatus{}
	errors := []error{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/api/v2/status", addr.ApiAddress),
			nil,
		)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d :%s", resp.StatusCode, resp.Status))
			continue
		}
		// unmarshal status response
		s := alertmanagerv2.AlertmanagerStatus{}
		if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
			errors = append(errors, err)
			continue
		}
		res = append(res, s)
	}
	return res, multierr.Combine(errors...)
}

func (c *client) Ready(ctx context.Context) error {
	addrs := c.MemberPeers()
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/-/ready", addr.ApiAddress), nil)
		if err != nil {
			return err
		}
		resp, err := c.Client.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}
	return nil
}

func (c *client) GetReceiver(ctx context.Context, id string) (alertmanagerv2.Receiver, error) {
	resp, err := c.ListReceivers(ctx)
	if err != nil {
		return alertmanagerv2.Receiver{}, err
	}
	for _, r := range resp {
		if r.Name == nil {
			continue
		}
		if *r.Name == id {
			return r, nil
		}
	}
	return alertmanagerv2.Receiver{}, fmt.Errorf("receiver not found")
}

func (c *client) ListReceivers(ctx context.Context) (recvs []alertmanagerv2.Receiver, err error) {
	addrs := c.MemberPeers()
	resolvedReceivers := 0
	errors := []error{}
	mappedReceivers := map[string]lo.Tuple2[int, alertmanagerv2.Receiver]{}

	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v2/receivers", addr.ApiAddress), nil)
		if err != nil {
			return recvs, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		rs := []alertmanagerv2.Receiver{}
		if err := json.NewDecoder(resp.Body).Decode(&rs); err != nil {
			return recvs, err
		}
		for _, r := range rs {
			if _, ok := mappedReceivers[*r.Name]; !ok {
				mappedReceivers[*r.Name] = lo.Tuple2[int, alertmanagerv2.Receiver]{A: 1, B: r}
			} else {
				mappedReceivers[*r.Name] = lo.Tuple2[int, alertmanagerv2.Receiver]{A: mappedReceivers[*r.Name].A + 1, B: r}
			}
		}
		resolvedReceivers += 1
	}
	if resolvedReceivers == 0 {
		return nil, multierr.Combine(errors...)
	}
	positiveResults := lo.PickBy(mappedReceivers, func(k string, v lo.Tuple2[int, alertmanagerv2.Receiver]) bool {
		return v.A == resolvedReceivers
	})

	return lo.MapToSlice(positiveResults, func(k string, v lo.Tuple2[int, alertmanagerv2.Receiver]) alertmanagerv2.Receiver {
		return v.B
	}), nil

}

func (c *client) ListAlerts(ctx context.Context) (alertmanagerv2.AlertGroups, error) {
	addrs := c.MemberPeers()
	n := len(addrs)
	res := alertmanagerv2.AlertGroups{}
	errors := []error{}
	dedupedAlerts := map[string]struct{}{}

	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/api/v2/alerts/groups",
				addr.ApiAddress,
			),
			nil,
		)
		if err != nil {
			return alertmanagerv2.AlertGroups{}, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			n -= 1
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			n -= 1
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		ags := alertmanagerv2.AlertGroups{}
		if err := json.NewDecoder(resp.Body).Decode(&ags); err != nil {
			return alertmanagerv2.AlertGroups{}, err
		}
		for _, ag := range ags {
			alerts := []*alertmanagerv2.GettableAlert{}
			for _, a := range ag.Alerts {

				key := fmt.Sprintf("%s-%s", a.Labels[alertingv1.NotificationPropertyOpniUuid], a.Labels[alertingv1.NotificationPropertyFingerprint])
				if _, ok := dedupedAlerts[key]; !ok {
					dedupedAlerts[key] = struct{}{}
					alerts = append(alerts, a)
				}
			}
			if len(alerts) > 0 {
				ag.Alerts = alerts
				res = append(res, ag)
			}
		}
	}
	if n == 0 {
		return res, multierr.Combine(errors...)
	}
	return res, multierr.Combine(errors...)
}

func allLabelsMatch(labels map[string]string, matchers []*labels.Matcher) bool {
	for _, matcher := range matchers {
		if !matcherMatchesLabels(matcher, labels) {
			return false
		}
	}
	return true
}

func matcherMatchesLabels(matcher *labels.Matcher, labels map[string]string) bool {
	for _, label := range labels {
		if matcher.Matches(label) {
			return true
		}
	}
	return false
}

func (c *client) GetAlert(ctx context.Context, matchLabels []*labels.Matcher) (alertmanagerv2.GettableAlerts, error) {
	alertGroups, err := c.ListAlerts(ctx)
	if err != nil {
		return alertmanagerv2.GettableAlerts{}, err
	}
	res := alertmanagerv2.GettableAlerts{}
	for _, ag := range alertGroups {
		for _, alert := range ag.Alerts {
			if allLabelsMatch(alert.Labels, matchLabels) {
				res = append(res, alert)
			}
		}
	}
	return res, nil
}

func toOpenApiTime(t time.Time) strfmt.DateTime {
	return strfmt.DateTime(t)
}

func (c *client) PostAlarm(
	ctx context.Context,
	alarm AlertObject,
) error {
	if err := alarm.Validate(); err != nil {
		return err
	}
	addrs := c.MemberPeers()
	n := len(addrs)
	errors := []error{}
	if _, ok := alarm.Labels[alertingv1.NotificationPropertyOpniUuid]; !ok {
		alarm.Labels[alertingv1.NotificationPropertyOpniUuid] = alarm.Id
	}
	if _, ok := alarm.Annotations[shared.OpniAlarmNameAnnotation]; !ok {
		alarm.Annotations[shared.OpniAlarmNameAnnotation] = alarm.Id
	}
	if _, ok := alarm.Labels[alertingv1.NotificationPropertyFingerprint]; !ok {
		alarm.Annotations[alertingv1.NotificationPropertyFingerprint] = string(fingerprint.Default())
	}
	for _, addr := range addrs {
		var b bytes.Buffer
		if err := json.NewEncoder(&b).Encode(alertmanagerv2.PostableAlerts{
			{
				StartsAt:    toOpenApiTime(time.Now()),
				Annotations: alarm.Annotations,
				Alert: alertmanagerv2.Alert{
					Labels: alarm.Labels,
				},
			},
		}); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf("%s/api/v2/alerts", addr.ApiAddress),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
	}
	if n == 0 {
		return multierr.Combine(errors...)
	}
	return nil
}

func (c *client) PostNotification(
	ctx context.Context,
	notification AlertObject,
) error {
	if err := notification.Validate(); err != nil {
		return err
	}
	addrs := c.MemberPeers()
	notifiedInstances := 0
	errors := []error{}
	var b bytes.Buffer
	if _, ok := notification.Labels[alertingv1.NotificationPropertyOpniUuid]; !ok {
		notification.Labels[alertingv1.NotificationPropertyOpniUuid] = notification.Id
	}
	if _, ok := notification.Labels[alertingv1.NotificationPropertyFingerprint]; !ok {
		notification.Labels[alertingv1.NotificationPropertyFingerprint] = string(fingerprint.Default())
	}
	t := time.Now()
	for _, addr := range addrs {
		if err := json.NewEncoder(&b).Encode(alertmanagerv2.PostableAlerts{
			{
				StartsAt:    toOpenApiTime(t),
				EndsAt:      toOpenApiTime(t.Add(2 * time.Minute)),
				Annotations: notification.Annotations,
				Alert: alertmanagerv2.Alert{
					Labels: notification.Labels,
				},
			},
		}); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf("%s/api/v2/alerts", addr.ApiAddress),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		notifiedInstances += 1
	}
	if notifiedInstances == 0 {
		return multierr.Combine(errors...)
	}
	return nil
}

func (c *client) ResolveAlert(ctx context.Context, alertObject AlertObject) error {
	if alertObject.Id == "" {
		return validation.Error("id is required")
	}
	addrs := c.MemberPeers()
	resolvedInstances := 0
	errors := []error{}
	var b bytes.Buffer
	if _, ok := alertObject.Labels[alertingv1.NotificationPropertyOpniUuid]; !ok {
		alertObject.Labels[alertingv1.NotificationPropertyOpniUuid] = alertObject.Id
	}
	t := time.Now()
	for _, addr := range addrs {
		if err := json.NewEncoder(&b).Encode(alertmanagerv2.PostableAlert{
			EndsAt:      toOpenApiTime(t.Add(-2 * time.Minute)),
			Annotations: alertObject.Annotations,
			Alert: alertmanagerv2.Alert{
				Labels: alertObject.Labels,
			},
		}); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf("%s/api/v2/alerts", addr.ApiAddress),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		resolvedInstances += 1
	}
	if resolvedInstances == 0 {
		return multierr.Combine(errors...)
	}
	return nil
}

func (c *client) ListSilences(ctx context.Context) (alertmanagerv2.GettableSilences, error) {
	addrs := c.MemberPeers()
	errors := []error{}
	resolvedSilences := 0
	mappedSilences := map[string]lo.Tuple2[int, *alertmanagerv2.GettableSilence]{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/api/v2/silences", addr.ApiAddress),
			nil,
		)
		if err != nil {
			return alertmanagerv2.GettableSilences{}, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		sil := alertmanagerv2.GettableSilences{}
		if err := json.NewDecoder(resp.Body).Decode(&sil); err != nil {
			return alertmanagerv2.GettableSilences{}, err
		}

		for _, s := range sil {
			if s.ID == nil {
				continue
			}
			if _, ok := mappedSilences[*s.ID]; !ok {
				mappedSilences[*s.ID] = lo.Tuple2[int, *alertmanagerv2.GettableSilence]{A: 1, B: s}
			} else {
				mappedSilences[*s.ID] = lo.Tuple2[int, *alertmanagerv2.GettableSilence]{A: mappedSilences[*s.ID].A + 1, B: s}
			}
		}
		resolvedSilences += 1
	}
	if resolvedSilences == 0 {
		return nil, multierr.Combine(errors...)
	}

	positiveResults := lo.PickBy(mappedSilences, func(k string, v lo.Tuple2[int, *alertmanagerv2.GettableSilence]) bool {
		return v.A == resolvedSilences
	})

	return lo.MapToSlice(positiveResults, func(k string, v lo.Tuple2[int, *alertmanagerv2.GettableSilence]) *alertmanagerv2.GettableSilence {
		return v.B
	}), nil
}

func allMatchersMatchSilence(labels alertmanagerv2.Matchers, matchers labels.Matchers) bool {
	for _, matcher := range matchers {
		matches := false
		for _, silenceMatcher := range labels {
			if matcherMatchesSilenceMatchers(*silenceMatcher, matcher) {
				matches = true
			}
		}
		if !matches {
			return false
		}
	}
	return true
}

func matcherMatchesSilenceMatchers(label alertmanagerv2.Matcher, matcher *labels.Matcher) bool {
	if matcher.Name == *label.Name &&
		matcher.Value == *label.Value {
		if matcher.Type == labels.MatchEqual &&
			lo.FromPtrOr(label.IsEqual, false) &&
			matcher.Value == lo.FromPtrOr(label.Value, "") {
			return true
		}
		if matcher.Type == labels.MatchRegexp &&
			lo.FromPtrOr(label.IsRegex, false) &&
			matcher.Value == lo.FromPtrOr(label.Value, "") {
			return true
		}
	}
	return false
}

func (c *client) GetSilence(ctx context.Context, matchers labels.Matchers) (alertmanagerv2.GettableSilences, error) {
	if len(matchers) == 0 {
		return nil, validation.Error("matchers are required")
	}
	silences, err := c.ListSilences(ctx)
	if err != nil {
		return nil, err
	}
	resSilences := alertmanagerv2.GettableSilences{}
	for _, silence := range silences {
		if len(silence.Matchers) != len(matchers) {
			continue
		}
		if allMatchersMatchSilence(silence.Matchers, matchers) {
			resSilences = append(resSilences, silence)
		}

	}
	return resSilences, nil
}

type postSilenceResponse struct {
	SilenceID string `json:"silenceId"`
}

func (c *client) PostSilence(ctx context.Context, alertingObjectId string, dur time.Duration, incomingSilenceId *string) (silenceId string, err error) {
	addrs := c.MemberPeers()
	var b bytes.Buffer
	t := time.Now()
	errors := []error{}
	silencedAlerts := len(addrs)
	var response postSilenceResponse
	for _, addr := range addrs {
		silence := alertmanagerv2.PostableSilence{
			ID: lo.FromPtrOr(incomingSilenceId, ""),
			Silence: alertmanagerv2.Silence{
				Comment:   lo.ToPtr("Silence created by Opni Admin"),
				CreatedBy: lo.ToPtr("Opni admin"),
				StartsAt:  lo.ToPtr(toOpenApiTime(t)),
				EndsAt:    lo.ToPtr(toOpenApiTime(t.Add(dur))),
				Matchers: alertmanagerv2.Matchers{
					{
						IsEqual: lo.ToPtr(true),
						IsRegex: lo.ToPtr(false),
						Name:    lo.ToPtr(alertingv1.NotificationPropertyOpniUuid),
						Value:   lo.ToPtr(alertingObjectId),
					},
				},
			},
		}
		if err := json.NewEncoder(&b).Encode(silence); err != nil {
			errors = append(errors, err)
			continue
		}
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf("%s/api/v2/silences", addr.ApiAddress),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			errors = append(errors, err)
			continue
		}
		silencedAlerts += 1
		break // silences only need to reach one instance since they are a stateful object
	}
	if silencedAlerts == 0 {
		return "", multierr.Combine(errors...)
	}
	return response.SilenceID, nil
}

func (c *client) DeleteSilence(ctx context.Context, silenceId string) error {
	addrs := c.MemberPeers()
	errors := []error{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			fmt.Sprintf("%s/api/v2/silence/%s", addr.ApiAddress, silenceId),
			nil,
		)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		resp, err := c.Client.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
	}
	return multierr.Combine(errors...)
}

func (c *client) ListAlarmMessages(ctx context.Context, listReq *alertingv1.ListAlarmMessageRequest) (*alertingv1.ListMessageResponse, error) {
	listReq.Sanitize()
	if err := listReq.Validate(); err != nil {
		return nil, err
	}
	b, err := protojson.Marshal(listReq)
	if err != nil {
		return nil, err
	}
	addrs := c.MemberPeers()
	n := len(addrs)
	mappedMsgs := map[string]*alertingv1.MessageInstance{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/alarms/list", addr.EmbeddedAddress),
			bytes.NewReader(b),
		)
		if err != nil {
			return nil, err
		}

		resp, err := c.Client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			n -= 1 // instance is unavailable
			continue
		}
		var listResp alertingv1.ListMessageResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, err
		}
		for _, msg := range listResp.Items {
			var id string
			if msg.Notification == nil {
				continue //FIXME: warn
			}
			if _, ok := msg.Notification.GetProperties()[alertingv1.NotificationPropertyOpniUuid]; !ok {
				continue //FIXME: warn
			}
			id = msg.Notification.GetProperties()[alertingv1.NotificationPropertyOpniUuid]
			if curMsg, ok := mappedMsgs[id]; !ok {
				mappedMsgs[id] = msg
			} else {
				if curMsg.LastUpdatedAt.AsTime().Before(msg.LastUpdatedAt.AsTime()) {
					mappedMsgs[id] = msg
				}
			}
			mappedMsgs[id] = msg
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("no instances were available when listing alarm messages")
	}
	retMsgs := lo.MapToSlice(mappedMsgs, func(k string, v *alertingv1.MessageInstance) *alertingv1.MessageInstance {
		return v
	})
	return &alertingv1.ListMessageResponse{
		Items: retMsgs,
	}, nil
}

func (c *client) ListNotificationMessages(
	ctx context.Context,
	listReq *alertingv1.ListNotificationRequest,
) (*alertingv1.ListMessageResponse, error) {
	listReq.Sanitize()
	if err := listReq.Validate(); err != nil {
		return nil, err
	}
	b, err := protojson.Marshal(listReq)
	if err != nil {
		return nil, err
	}
	addrs := c.MemberPeers()
	n := len(addrs)
	mappedMsgs := map[string]*alertingv1.MessageInstance{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/notifications/list", addr.EmbeddedAddress),
			bytes.NewReader(b),
		)
		if err != nil {
			return nil, err
		}

		resp, err := c.Client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			n -= 1 // instance is unavailable
			continue
		}
		var listResp alertingv1.ListMessageResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, err
		}
		for _, msg := range listResp.Items {
			var id string
			if msg.Notification == nil {
				continue //FIXME: warn
			}
			if _, ok := msg.Notification.GetProperties()[alertingv1.NotificationPropertyOpniUuid]; !ok {
				continue //FIXME: warn
			}
			id = msg.Notification.GetProperties()[alertingv1.NotificationPropertyOpniUuid]
			if curMsg, ok := mappedMsgs[id]; !ok {
				mappedMsgs[id] = msg
			} else {
				if curMsg.LastUpdatedAt.AsTime().Before(msg.LastUpdatedAt.AsTime()) {
					mappedMsgs[id] = msg
				}
			}
			mappedMsgs[id] = msg
		}
	}
	if n == 0 {
		return nil, fmt.Errorf("no instances were available when listing alarm messages")
	}
	retMsgs := lo.MapToSlice(mappedMsgs, func(k string, v *alertingv1.MessageInstance) *alertingv1.MessageInstance {
		return v
	})
	return &alertingv1.ListMessageResponse{
		Items: retMsgs,
	}, nil
}
