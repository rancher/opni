package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/go-openapi/strfmt"
	alertmanagerv2 "github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/alertmanager/pkg/labels"
	"github.com/rancher/opni/pkg/alerting/fingerprint"
	"github.com/rancher/opni/pkg/alerting/message"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/validation"
	"github.com/samber/lo"
	"go.uber.org/multierr"
	"google.golang.org/protobuf/encoding/protojson"
)

func DefaultOptions() *ClientOptions {
	return &ClientOptions{
		proxyClient:         http.Client{},
		queryClient:         http.Client{},
		alertmanagerAddress: fmt.Sprintf("%s:9093", shared.AlertmanagerService),
		querierAddress:      fmt.Sprintf("%s:3000", shared.AlertmanagerService),
		proxyAddress:        fmt.Sprintf("%s:9093", shared.AlertmanagerService),
	}
}

type TLSClientConfig struct {
	// Path to the server CA certificate.
	ServerCA string
	// Path to the client CA certificate (not needed in all cases).
	ClientCA string
	// Path to the certificate used for client-cert auth.
	ClientCert string
	// Path to the private key used for client-cert auth.
	ClientKey string
}

func (t *TLSClientConfig) Init() (*tls.Config, error) {
	clientCert, err := tls.LoadX509KeyPair(t.ClientCert, t.ClientKey) //alertingClientCert, alertingClientKey)
	if err != nil {
		return nil, err
	}

	serverCaPool := x509.NewCertPool()
	serverCaData, err := os.ReadFile(t.ServerCA)
	if err != nil {
		return nil, err
	}

	if ok := serverCaPool.AppendCertsFromPEM(serverCaData); !ok {
		return nil, fmt.Errorf("failed to load alerting server CA")
	}

	clientCaPool := x509.NewCertPool()
	clientCaData, err := os.ReadFile(t.ClientCA)
	if err != nil {
		return nil, err
	}

	if ok := clientCaPool.AppendCertsFromPEM(clientCaData); !ok {
		return nil, fmt.Errorf("failed to load alerting client Ca")
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{clientCert},
		ClientCAs:    clientCaPool,
		RootCAs:      serverCaPool,
	}, nil
}

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
type Client struct {
	*ClientOptions

	peerMu     sync.RWMutex
	knownPeers []AlertingPeer
}

var _ AlertingClient = (*Client)(nil)

type ClientOptions struct {
	proxyClient         http.Client
	queryClient         http.Client
	alertmanagerAddress string
	proxyAddress        string
	querierAddress      string
	tlsConfig           *tls.Config
}

func (o *ClientOptions) Apply(opts ...ClientOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func WithAlertManagerAddress(addr string) ClientOption {
	return func(o *ClientOptions) {
		o.alertmanagerAddress = addr
	}
}

func WithProxyAddress(addr string) ClientOption {
	return func(o *ClientOptions) {
		o.proxyAddress = addr
	}
}

func WithQuerierAddress(addr string) ClientOption {
	return func(o *ClientOptions) {
		o.querierAddress = addr
	}
}

func WithProxyHttpClient(client http.Client) ClientOption {
	return func(o *ClientOptions) {
		o.proxyClient = client
	}
}

func WithTLSConfig(config *tls.Config) ClientOption {
	return func(o *ClientOptions) {
		o.tlsConfig = config
	}
}

func (o *ClientOptions) Validate(
	proxyScheme string,
	queryScheme string,
) error {
	if _, err := url.Parse(proxyScheme + "://" + o.alertmanagerAddress); err != nil {
		return err
	}
	if _, err := url.Parse(proxyScheme + "://" + o.proxyAddress); err != nil {
		return err
	}
	if _, err := url.Parse(queryScheme + "://" + o.querierAddress); err != nil {
		return err
	}
	return nil
}

type ClientOption func(*ClientOptions)

func NewClient(
	opts ...ClientOption,
) (*Client, error) {
	options := DefaultOptions()
	options.Apply(opts...)
	c := &Client{
		knownPeers: []AlertingPeer{},
		peerMu:     sync.RWMutex{},
	}

	if err := options.Validate(
		c.proxyScheme(),
		c.queryScheme(),
	); err != nil {
		return nil, err
	}
	httptransport := &http.Transport{
		TLSClientConfig: options.tlsConfig,
	}
	options.proxyClient.Transport = httptransport

	c.ClientOptions = options
	return c, nil
}

// A typical HA setup of prometheus operator assumes that alert sources
// (in particular Prometheus), triggers api calls to all known peers
// and alert status & deduplication will be handled by its memberlisted nflog.
//
// In our alerting client abstraction
type AlertingClient interface {
	MemberlistClient() MemberlistClient
	ControlClient() ControlClient
	StatusClient() StatusClient
	ConfigClient() ConfigClient
	AlertClient() AlertClient
	QueryClient() QueryClient
	SilenceClient() SilenceClient
	ProxyClient() ProxyClient
	Clone() AlertingClient
}

type ProxyClient interface {
	ProxyURL() *url.URL
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

func (c *Client) Clone() AlertingClient {
	cl := &Client{
		ClientOptions: c.ClientOptions,
		peerMu:        sync.RWMutex{},
		knownPeers:    []AlertingPeer{},
	}
	cl.SetKnownPeers(c.MemberPeers())
	return cl
}

func (c *Client) MemberlistClient() MemberlistClient {
	return c
}

func (c *Client) ControlClient() ControlClient {
	return c
}

func (c *Client) StatusClient() StatusClient {
	return c
}

func (c *Client) ConfigClient() ConfigClient {
	return c
}

func (c *Client) AlertClient() AlertClient {
	return c
}

func (c *Client) QueryClient() QueryClient {
	return c
}

func (c *Client) SilenceClient() SilenceClient {
	return c
}

func (c *Client) ProxyClient() ProxyClient {
	return c
}

func (c *Client) proxyScheme() string {
	return "https"
}

func (c *Client) queryScheme() string {
	return "http"
}

func (c *Client) alertmanagerTarget(addr string) string {
	target := url.URL{
		Scheme: c.proxyScheme(),
		Host:   addr,
	}
	return target.String()
}

func (c *Client) querierTarget(addr string) string {
	target := url.URL{
		Scheme: c.queryScheme(),
		Host:   addr,
	}
	return target.String()
}

func (c *Client) proxyTarget() string {
	proxyTarget := url.URL{
		Scheme: c.proxyScheme(),
		Host:   c.proxyAddress,
	}
	return proxyTarget.String()
}

func (c *Client) Reload(ctx context.Context) error {
	addrs := c.MemberPeers()
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(ctx,
			http.MethodPost,
			fmt.Sprintf("%s/-/reload", c.alertmanagerTarget(addr.ApiAddress)),
			nil)
		if err != nil {
			return err
		}
		resp, err := c.proxyClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}
	return nil
}

func (c *Client) ProxyURL() *url.URL {
	// TODO : maybe not panic here, maybe its ok to surface an error
	// however this a pretty bad configuration error
	return util.Must(url.Parse(c.proxyTarget()))
}

func (c *Client) MemberPeers() []AlertingPeer {
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

func (c *Client) SetKnownPeers(peers []AlertingPeer) {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()
	c.knownPeers = peers
}

func (c *Client) Status(ctx context.Context) ([]alertmanagerv2.AlertmanagerStatus, error) {
	addrs := c.MemberPeers()
	res := []alertmanagerv2.AlertmanagerStatus{}
	errors := []error{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/api/v2/status", c.alertmanagerTarget(addr.ApiAddress)),
			nil,
		)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.proxyClient.Do(req)
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

func (c *Client) Ready(ctx context.Context) error {
	addrs := c.MemberPeers()
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/-/ready", c.alertmanagerTarget(addr.ApiAddress)), nil)
		if err != nil {
			return err
		}
		resp, err := c.proxyClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	}
	return nil
}

func (c *Client) GetReceiver(ctx context.Context, id string) (alertmanagerv2.Receiver, error) {
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

func (c *Client) ListReceivers(ctx context.Context) (recvs []alertmanagerv2.Receiver, err error) {
	addrs := c.MemberPeers()
	resolvedReceivers := 0
	errors := []error{}
	mappedReceivers := map[string]lo.Tuple2[int, alertmanagerv2.Receiver]{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v2/receivers", c.alertmanagerTarget(addr.ApiAddress)), nil)
		if err != nil {
			return recvs, err
		}
		req.Header.Set("Accept", "application/json")

		resp, err := c.proxyClient.Do(req)
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
		resolvedReceivers++
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

func (c *Client) ListAlerts(ctx context.Context) (alertmanagerv2.AlertGroups, error) {
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
				c.alertmanagerTarget(addr.ApiAddress),
			),
			nil,
		)
		if err != nil {
			return alertmanagerv2.AlertGroups{}, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.proxyClient.Do(req)
		if err != nil {
			n--
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			n--
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

				key := fmt.Sprintf("%s-%s", a.Labels[message.NotificationPropertyOpniUuid], a.Labels[message.NotificationPropertyFingerprint])
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

func (c *Client) GetAlert(ctx context.Context, matchLabels []*labels.Matcher) (alertmanagerv2.GettableAlerts, error) {
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

func ToOpenApiTime(t time.Time) strfmt.DateTime {
	return strfmt.DateTime(t)
}

func (c *Client) PostAlarm(
	ctx context.Context,
	alarm AlertObject,
) error {
	if err := alarm.Validate(); err != nil {
		return err
	}
	addrs := c.MemberPeers()
	n := len(addrs)
	errors := []error{}
	if _, ok := alarm.Labels[message.NotificationPropertyOpniUuid]; !ok {
		alarm.Labels[message.NotificationPropertyOpniUuid] = alarm.Id
	}
	if _, ok := alarm.Annotations[message.NotificationContentAlarmName]; !ok {
		alarm.Annotations[message.NotificationContentAlarmName] = alarm.Id
	}
	if _, ok := alarm.Labels[message.NotificationPropertyFingerprint]; !ok {
		alarm.Annotations[message.NotificationPropertyFingerprint] = string(fingerprint.Default())
	}
	for _, addr := range addrs {
		var b bytes.Buffer
		if err := json.NewEncoder(&b).Encode(alertmanagerv2.PostableAlerts{
			{
				StartsAt:    ToOpenApiTime(time.Now()),
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
			fmt.Sprintf("%s/api/v2/alerts", c.alertmanagerTarget(addr.ApiAddress)),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.proxyClient.Do(req)
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
	if n == 0 {
		return multierr.Combine(errors...)
	}
	return nil
}

func (c *Client) PostNotification(
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
	if _, ok := notification.Labels[message.NotificationPropertyOpniUuid]; !ok {
		notification.Labels[message.NotificationPropertyOpniUuid] = notification.Id
	}
	if _, ok := notification.Labels[message.NotificationPropertyFingerprint]; !ok {
		notification.Labels[message.NotificationPropertyFingerprint] = string(fingerprint.Default())
	}
	t := time.Now()
	for _, addr := range addrs {
		if err := json.NewEncoder(&b).Encode(alertmanagerv2.PostableAlerts{
			{
				StartsAt:    ToOpenApiTime(t),
				EndsAt:      ToOpenApiTime(t.Add(2 * time.Minute)),
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
			fmt.Sprintf("%s/api/v2/alerts", c.alertmanagerTarget(addr.ApiAddress)),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := c.proxyClient.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		notifiedInstances++
	}
	if notifiedInstances == 0 {
		return multierr.Combine(errors...)
	}
	return nil
}

func (c *Client) ResolveAlert(ctx context.Context, alertObject AlertObject) error {
	if alertObject.Id == "" {
		return validation.Error("id is required")
	}
	addrs := c.MemberPeers()
	resolvedInstances := 0
	errors := []error{}
	var b bytes.Buffer
	if _, ok := alertObject.Labels[message.NotificationPropertyOpniUuid]; !ok {
		alertObject.Labels[message.NotificationPropertyOpniUuid] = alertObject.Id
	}
	t := time.Now()
	for _, addr := range addrs {
		if err := json.NewEncoder(&b).Encode(alertmanagerv2.PostableAlert{
			EndsAt:      ToOpenApiTime(t.Add(-2 * time.Minute)),
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
			fmt.Sprintf("%s/api/v2/alerts", c.alertmanagerTarget(addr.ApiAddress)),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.proxyClient.Do(req)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errors = append(errors, fmt.Errorf("unexpected status code: %d", resp.StatusCode))
			continue
		}
		resolvedInstances++
	}
	if resolvedInstances == 0 {
		return multierr.Combine(errors...)
	}
	return nil
}

func (c *Client) ListSilences(ctx context.Context) (alertmanagerv2.GettableSilences, error) {
	addrs := c.MemberPeers()
	errors := []error{}
	resolvedSilences := 0
	mappedSilences := map[string]lo.Tuple2[int, *alertmanagerv2.GettableSilence]{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodGet,
			fmt.Sprintf("%s/api/v2/silences", c.alertmanagerTarget(addr.ApiAddress)),
			nil,
		)
		if err != nil {
			return alertmanagerv2.GettableSilences{}, err
		}
		req.Header.Set("Accept", "application/json")
		resp, err := c.proxyClient.Do(req)
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
		resolvedSilences++
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

func (c *Client) GetSilence(ctx context.Context, matchers labels.Matchers) (alertmanagerv2.GettableSilences, error) {
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

func (c *Client) PostSilence(ctx context.Context, alertingObjectId string, dur time.Duration, incomingSilenceId *string) (silenceId string, err error) {
	addrs := c.MemberPeers()
	var b bytes.Buffer
	t := time.Now()
	reqs := make([]*http.Request, len(addrs))
	for i, addr := range addrs {
		reqBody := alertmanagerv2.PostableSilence{
			ID: lo.FromPtrOr(incomingSilenceId, ""),
			Silence: alertmanagerv2.Silence{
				Comment:   lo.ToPtr("Silence created by Opni Admin"),
				CreatedBy: lo.ToPtr("Opni admin"),
				StartsAt:  lo.ToPtr(ToOpenApiTime(t)),
				EndsAt:    lo.ToPtr(ToOpenApiTime(t.Add(dur))),
				Matchers: alertmanagerv2.Matchers{
					{
						IsEqual: lo.ToPtr(true),
						IsRegex: lo.ToPtr(false),
						Name:    lo.ToPtr(message.NotificationPropertyOpniUuid),
						Value:   lo.ToPtr(alertingObjectId),
					},
				},
			},
		}
		if err := json.NewEncoder(&b).Encode(reqBody); err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			fmt.Sprintf("%s/api/v2/silences", c.alertmanagerTarget(addr.ApiAddress)),
			bytes.NewReader(b.Bytes()),
		)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		reqs[i] = req
	}
	silenceResp, err := AtMostOne[postSilenceResponse](ctx, c.proxyClient, reqs)
	if err != nil {
		return "", err
	}
	return silenceResp.SilenceID, nil
}

func (c *Client) DeleteSilence(ctx context.Context, silenceId string) error {
	addrs := c.MemberPeers()
	errors := []error{}
	for _, addr := range addrs {
		req, err := http.NewRequestWithContext(
			ctx,
			http.MethodDelete,
			fmt.Sprintf("%s/api/v2/silence/%s", c.alertmanagerTarget(addr.ApiAddress), silenceId),
			nil,
		)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		resp, err := c.proxyClient.Do(req)
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

func (c *Client) ListAlarmMessages(ctx context.Context, listReq *alertingv1.ListAlarmMessageRequest) (*alertingv1.ListMessageResponse, error) {
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
			http.MethodPost,
			fmt.Sprintf("%s/alarms/list", c.querierTarget(addr.EmbeddedAddress)),
			bytes.NewReader(b),
		)
		if err != nil {
			return nil, err
		}

		resp, err := c.queryClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			n-- // instance is unavailable
			continue
		}
		var listResp alertingv1.ListMessageResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, err
		}
		for _, msg := range listResp.Items {
			var id string
			if msg.Notification == nil {
				continue
			}
			if _, ok := msg.Notification.GetProperties()[message.NotificationPropertyOpniUuid]; !ok {
				continue
			}
			id = msg.Notification.GetProperties()[message.NotificationPropertyOpniUuid]
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

func (c *Client) ListNotificationMessages(
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
			http.MethodPost,
			fmt.Sprintf("%s/notifications/list", c.querierTarget(addr.EmbeddedAddress)),
			bytes.NewReader(b),
		)
		if err != nil {
			return nil, err
		}

		resp, err := c.queryClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			n-- // instance is unavailable
			continue
		}
		var listResp alertingv1.ListMessageResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			return nil, err
		}
		for _, msg := range listResp.Items {
			var id string
			if msg.Notification == nil {
				continue
			}
			if _, ok := msg.Notification.GetProperties()[message.NotificationPropertyOpniUuid]; !ok {
				continue
			}
			id = msg.Notification.GetProperties()[message.NotificationPropertyOpniUuid]
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
