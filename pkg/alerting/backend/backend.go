package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strings"
	"time"

	backoffv2 "github.com/lestrrat-go/backoff/v2"
	"github.com/rancher/opni/pkg/alerting/routing"
	"github.com/rancher/opni/pkg/logger"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

const (
	GET    = "GET"
	POST   = "POST"
	DELETE = "DELETE"
	v2     = "/api/v2"
	v1     = "/api/v1"
)

type AlertManagerApiOptions struct {
	client        *http.Client
	backoff       *backoffv2.Policy
	expectClosure func(*http.Response) error
	body          io.Reader
	values        url.Values
	logger        *zap.SugaredLogger
}

type AlertManagerApiOption func(*AlertManagerApiOptions)

func (o *AlertManagerApiOptions) apply(opts ...AlertManagerApiOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func NewDefaultAlertManagerOptions() *AlertManagerApiOptions {
	return &AlertManagerApiOptions{
		client:  http.DefaultClient,
		backoff: nil,
		expectClosure: func(resp *http.Response) error {
			return nil
		},
		body:   nil,
		values: nil,
		logger: logger.NewPluginLogger().Named("alerting"),
	}
}

func WithHttpClient(client *http.Client) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		o.client = client
	}
}

func WithRetrier(retrier backoffv2.Policy) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		o.backoff = &retrier
	}
}

func WithExpectClosure(expectClosure func(*http.Response) error) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		o.expectClosure = expectClosure
	}
}

func WithRequestBody(body io.Reader) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		o.body = body
	}
}

func WithURLValues(values url.Values) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		o.values = values
	}
}

func WithLogger(logger *zap.SugaredLogger) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		o.logger = logger
	}
}

// providing a silence id here indicates we are updating an existing silence
func WithPostSilenceBody(conditionId string, duration time.Duration, silenceId *string) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		p := &PostableSilence{}
		p.WithCondition(conditionId)
		p.WithDuration(duration)
		if err := p.Must(); err != nil {
			panic(fmt.Errorf("malformed silence body :%s", err))
		}
		if silenceId != nil {
			p.WithSilenceId(*silenceId)
		}
		b, err := json.Marshal(p)
		if err != nil {
			panic(fmt.Errorf("malformed silence body :%s", err))
		}
		o.body = strings.NewReader(string(b))
	}
}

func WithPostAlertBody(conditionId string, annotations map[string]string) AlertManagerApiOption {
	return func(o *AlertManagerApiOptions) {
		var alertsArr []*PostableAlert
		alert := &PostableAlert{}
		alert.WithCondition(conditionId)
		for annotationName, annotationValue := range annotations {
			alert.WithRuntimeInfo(annotationName, annotationValue)
		}
		alertsArr = append(alertsArr, alert)
		for _, alert := range alertsArr {
			if err := alert.Must(); err != nil {
				panic(fmt.Errorf("invalid alert req in post alert body %s", err))
			}
		}
		b, err := json.Marshal(alertsArr)
		if err != nil {
			panic(fmt.Errorf("invalid alert req in post alert body %s", err))
		}
		o.body = strings.NewReader(string(b))
	}
}

type AlertManagerAPI struct {
	*AlertManagerApiOptions
	Endpoint string
	Api      string
	Route    string
	Verb     string
	ctx      context.Context
}

func (a *AlertManagerAPI) DoRequest() error {
	if a.backoff != nil {
		b := (*a.backoff).Start(a.ctx)
		lastRetrierError := fmt.Errorf("unknwon error")
		numRetries := 0
		for backoffv2.Continue(b) {
			if err := a.doRequest(); err == nil {
				return nil
			} else {
				lastRetrierError = err
			}
			numRetries += 1
		}
		return fmt.Errorf("failed to complete request after retrier timeout (%d retries) : %s", numRetries, lastRetrierError)
	} else {
		return a.doRequest()
	}
}

func (a *AlertManagerAPI) doRequest() error {
	lg := a.logger.With("action", "DoRequest")
	req, err := http.NewRequestWithContext(
		a.ctx,
		a.Verb,
		a.ConstructHTTP(),
		a.body,
	)
	if err != nil {
		lg.Error(
			zap.Error(err),
		)
		return err
	}
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if a.values != nil {
		req.URL.RawQuery = a.values.Encode()
	}
	resp, err := a.client.Do(req)
	if err != nil {
		lg.Error(
			zap.Error(err),
		)
		return err
	}
	if err := a.expectClosure(resp); err != nil {
		lg.Error(
			zap.Error(err),
		)
		return err
	}
	return nil
}

// WithAPIV2
// ## OpenAPI reference
// https://github.com/prometheus/alertmanager/blob/main/api/v2/openapi.yaml
func (a *AlertManagerAPI) WithAPIV2() *AlertManagerAPI {
	a.Api = v2
	return a
}

// WithAPIV1
// ## Reference
// https://prometheus.io/docs/alerting/latest/clients/
func (a *AlertManagerAPI) WithAPIV1() *AlertManagerAPI {
	a.Api = v1
	return a
}

func NewAlertManagerReloadClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return &AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/-/reload",
		Verb:                   POST,
		ctx:                    ctx,
	}
}

func NewAlertManagerReadyClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return &AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/-/ready",
		Verb:                   GET,
		ctx:                    ctx,
	}
}

func NewAlertManagerReceiversClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return (&AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/receivers",
		Verb:                   GET,
		ctx:                    ctx,
	}).WithAPIV2()
}

func NewAlertManagerStatusClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return (&AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/status",
		Verb:                   GET,
		ctx:                    ctx,
	}).WithAPIV2()
}

func NewAlertManagerGetAlertsClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return (&AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/alerts",
		Verb:                   GET,
		ctx:                    ctx,
	}).WithAPIV2()
}

func NewAlertManagerPostAlertClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return (&AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/alerts",
		Verb:                   POST,
		ctx:                    ctx,
	}).WithAPIV2()
}
func NewAlertManagerPostSilenceClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return (&AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/silence",
		Verb:                   POST,
		ctx:                    ctx,
	}).WithAPIV2()
}

func NewAlertManagerGetSilenceClient(endpoint string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	return (&AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  "/silence",
		Verb:                   GET,
		ctx:                    ctx,
	}).WithAPIV2()
}

func NewAlertManagerDeleteSilenceClient(endpoint, silenceId string, ctx context.Context, opts ...AlertManagerApiOption) *AlertManagerAPI {
	options := NewDefaultAlertManagerOptions()
	options.apply(opts...)
	if silenceId == "" {
		panic("silenceId cannot be empty")
	}
	return (&AlertManagerAPI{
		AlertManagerApiOptions: options,
		Endpoint:               endpoint,
		Route:                  fmt.Sprintf("/silence/%s", silenceId),
		Verb:                   DELETE,
		ctx:                    ctx,
	}).WithAPIV2()
}

func (a *AlertManagerAPI) Construct() string {
	return a.Endpoint + path.Join(a.Api, a.Route)
}

func (a *AlertManagerAPI) ConstructHTTP() string {
	tempRes := a.Construct()
	if !strings.HasPrefix(tempRes, "http://") {
		return fmt.Sprintf("http://%s", tempRes)
	}
	return tempRes
}

func (a *AlertManagerAPI) ConstructHTTPS() string {
	tempRes := a.Construct()
	if !strings.HasPrefix("http://", tempRes) {
		return fmt.Sprintf("https://%s", tempRes)
	} else if strings.HasPrefix("http://", tempRes) {
		return strings.Replace(tempRes, "http://", "https://", 1)
	} else {
		return tempRes
	}
}

func NewExpectStatusOk() func(*http.Response) error {
	return func(resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		return nil
	}
}

// FIXME: there has to be a way to do this that will work
func NewExpectConfigEqual(expectedConfig string) func(*http.Response) error {
	// newConfig := newConfig
	return func(resp *http.Response) error {
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		result := gjson.Get(string(body), "config.original")
		if !result.Exists() {
			return fmt.Errorf("config.original not found in response body")
		}
		r1 := &routing.RoutingTree{}
		r2 := &routing.RoutingTree{}
		lg := logger.NewForPlugin().Named("alerting")
		err = r1.Parse(result.String())
		if err != nil {
			return err
		} else {
			lg.Debug("%v", r1)
		}
		err = r2.Parse(expectedConfig)
		if err != nil {
			return err
		} else {
			lg.Debug("%v", r2)
		}
		// cannot compare entire structs since AlertManager does invisble maintenance on the config
		if !reflect.DeepEqual(r1.Receivers, r2.Receivers) {
			return fmt.Errorf("current alertmanager receivers differ from expected receivers")
		}
		if !reflect.DeepEqual(r1.Route, r2.Route) {
			return fmt.Errorf("current alertmanager route differ from expected route")
		}
		return nil
	}
}

// Apis to be called in succession, if failed restart the entire pipeline
func NewApiPipline(ctx context.Context, apis []*AlertManagerAPI, chainRetrier *backoffv2.Policy) error {
	if chainRetrier == nil {
		return newApiPipeline(apis)
	} else {
		b := (*chainRetrier).Start(ctx)
		lastRetrierError := fmt.Errorf("unkown error")
		numRetries := 0
		for backoffv2.Continue(b) {
			if err := newApiPipeline(apis); err == nil {
				return nil
			} else {
				lastRetrierError = err
			}
			numRetries += 1
		}
		return fmt.Errorf("api pipeline failed with backoff retrier timeout (%d retries) : %s", numRetries, lastRetrierError)
	}
}

func newApiPipeline(apis []*AlertManagerAPI) error {
	for _, api := range apis {
		if err := api.DoRequest(); err != nil {
			return err
		}
	}
	return nil
}
