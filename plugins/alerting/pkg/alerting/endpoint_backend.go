package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rancher/opni/pkg/util/future"

	"github.com/phayes/freeport"
	cfg "github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/template"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util/waitctx"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var configPersistMu = &sync.Mutex{}

const NoSmartHostSet = "no global SMTP smarthost set"

var FieldNotFound = regexp.MustCompile("line [0-9]+: field .* not found in type config.plain")

const (
	GET    = "GET"
	POST   = "POST"
	DELETE = "DELETE"
	v2     = "/api/v2"
	v1     = "/api/v1"
)

func doRolloutRestart(ctx context.Context, c client.Client, dep *appsv1.StatefulSet) error {
	patchObj := client.StrategicMergeFrom(dep)
	err := c.Patch(
		ctx,
		dep,
		patchObj,
		&client.PatchOptions{
			FieldManager: "kubectl-rollout",
		},
	)
	return err
}

type RuntimeEndpointBackend interface {
	Fetch(ctx context.Context, p *Plugin, key string) (string, error)
	Put(ctx context.Context, p *Plugin, key string, data *ConfigMapData) error
	Reload(ctx context.Context, p *Plugin) error
	Port() int
}

// LocalEndpointBackend implements alerting.RuntimeEndpointBackend
//
// Only used for test:env and non kubernetes environments
type LocalEndpointBackend struct {
	configFilePath string
	ctx            context.Context
	cancelFunc     context.CancelFunc
	p              *Plugin
	port           int
}

func (b *LocalEndpointBackend) Start() {
	b.ctx, b.cancelFunc = context.WithCancel(b.p.ctx)
	port, err := freeport.GetFreePort()
	fmt.Printf("AlertManager port %d", port)
	if err != nil {
		panic(err)
	}
	lg := b.p.logger
	if err != nil {
		panic(err)
	}
	//TODO: fixme relative path only works for one of tests or mage test:env, but not both
	amBin := path.Join("testbin/bin", "alertmanager")
	defaultArgs := []string{
		fmt.Sprintf("--config.file=%s", b.configFilePath),
		fmt.Sprintf("--web.listen-address=:%d", port),
		"--storage.path=/tmp/data",
		"--log.level=debug",
	}
	cmd := exec.CommandContext(b.ctx, amBin, defaultArgs...)
	lg.With("port", port).Info("Starting AlertManager")
	session, err := testutil.StartCmd(cmd)
	if err != nil {
		if !errors.Is(b.ctx.Err(), context.Canceled) {
			panic(err)
		} else {
			return
		}
	}
	for b.ctx.Err() == nil {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/-/ready", port))
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(time.Second)
	}
	lg.With("address", fmt.Sprintf("http://localhost:%d", port)).Info("AlertManager started")
	waitctx.Permissive.Go(b.ctx, func() {
		<-b.ctx.Done()
		cmd, _ := session.G()
		if cmd != nil {
			cmd.Signal(os.Signal(syscall.SIGTERM))
		}
	})
	b.port = port
}

func (b *LocalEndpointBackend) Fetch(
	ctx context.Context, p *Plugin, key string) (string, error) {
	data, err := os.ReadFile(b.configFilePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (b *LocalEndpointBackend) Put(
	ctx context.Context, p *Plugin, key string, data *ConfigMapData) error {
	loopError := ReconcileInvalidStateLoop(time.Duration(time.Second*10), data, p.logger)
	if loopError != nil {
		return shared.WithInternalServerError(fmt.Sprintf("failed to reconcile config : %s", loopError))
	}
	applyData, err := data.Marshal()
	if err != nil {
		return err
	}
	err = os.WriteFile(b.configFilePath, applyData, 0644)
	return err
}

func (b *LocalEndpointBackend) Port() int {
	return b.port
}

func (b *LocalEndpointBackend) Reload(ctx context.Context,
	p *Plugin) error {
	retries := 10
	if b.cancelFunc != nil {
		if b.port == 0 {
			panic("invalid port")
		}
		webClient := &AlertManagerAPI{
			Endpoint: fmt.Sprintf("localhost:%d", b.port),
			Route:    "/-/ready",
			Verb:     GET,
		}
		b.p.logger.Info("Shutting down Alertmanager for reload...")
		b.cancelFunc()
		for i := 0; i < retries; i++ {
			resp, err := http.Get(webClient.ConstructHTTP())
			if err != nil {
				break
			}
			if resp.StatusCode == http.StatusNotFound {
				break
			}
			time.Sleep(time.Second)
		}
	}
	b.Start()
	newWebClient := &AlertManagerAPI{
		Endpoint: fmt.Sprintf("localhost:%d", b.port),
		Route:    "/-/ready",
		Verb:     GET,
	}
	for i := 0; i < retries; i++ {
		resp, err := http.Get(newWebClient.ConstructHTTP())
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			time.Sleep(time.Second)
		} else {
			backend, err := p.endpointBackend.GetContext(ctx)
			if err != nil {
				return fmt.Errorf("failed to get alerting backend when reloading alertmanager locally : %s", err)
			}
			p.endpointBackend.Set(backend)
			options, err := p.alertingOptions.GetContext(ctx)
			if err != nil {
				return fmt.Errorf("failed to get alerting options when reloading AM locally : %s", err)
			}
			options.Endpoints = []string{fmt.Sprintf("http://localhost:%d", b.Port())}
			b.p.logger.Debug(fmt.Sprintf("Setting alert manager address to %s", options.Endpoints[0]))
			p.alertingOptions = future.New[AlertingOptions]()
			p.alertingOptions.Set(options)
			return nil
		}
	}
	return fmt.Errorf("failed to reload local test backend for opni-alerting")
}

// K8sEndpointBackend implements alerting.RuntimeEndpointBackend
type K8sEndpointBackend struct {
	client client.Client
}

func (b *K8sEndpointBackend) Fetch(
	ctx context.Context, p *Plugin, key string) (string, error) {
	name := p.alertingOptions.Get().ConfigMap
	cfgMap := &corev1.ConfigMap{}
	err := b.client.Get(ctx, client.ObjectKey{
		Namespace: "", // check in all?
		Name:      name,
	}, cfgMap)

	if err != nil || cfgMap == nil {
		returnErr := shared.WithInternalServerError(
			fmt.Sprintf("K8s runtime error, config map: %s not found: %s",
				name,
				err),
		)
		return "", returnErr
	}

	if _, ok := cfgMap.Data[key]; !ok {
		return "", shared.WithInternalServerError(
			fmt.Sprintf(
				"K8s runtime error, config map : %s key : %s not found",
				name,
				key,
			),
		)
	}
	return cfgMap.Data[key], nil
}

func (b *K8sEndpointBackend) Put(ctx context.Context, p *Plugin, key string, data *ConfigMapData) error {
	reconcileLoopError := ReconcileInvalidStateLoop(time.Duration(time.Second*10), data, p.logger)
	if reconcileLoopError != nil {
		return shared.WithInternalServerErrorf(fmt.Sprintf("%s", reconcileLoopError))
	}
	configPersistMu.Lock()
	defer configPersistMu.Unlock()
	name := p.alertingOptions.Get().ConfigMap
	cfgMap := &corev1.ConfigMap{}
	err := b.client.Get(ctx, client.ObjectKey{
		Namespace: "", // check in all?
		Name:      name,
	}, cfgMap)

	if err != nil || cfgMap == nil {
		returnErr := shared.WithInternalServerError(
			fmt.Sprintf("K8s runtime error, config map : %s not found : %s",
				name,
				err),
		)
		return returnErr
	}
	applyData, err := data.Marshal()
	if err != nil {
		return err
	}
	cfgMap.Data[key] = string(applyData)

	err = b.client.Update(ctx, cfgMap)
	if err != nil {
		return shared.WithInternalServerError(
			fmt.Sprintf("Failed to update alertmanager configmap %s: %s", name, err),
		)
	}

	return nil
}

func (b *K8sEndpointBackend) Reload(ctx context.Context, p *Plugin) error {
	name := p.alertingOptions.Get().StatefulSet
	namespace := p.alertingOptions.Get().Namespace
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	err := doRolloutRestart(ctx, b.client, statefulSet)
	if err != nil {
		return shared.WithInternalServerError(
			fmt.Sprintf("K8s runtime error, statefulset : %s restart failed : %s",
				name,
				err,
			),
		)
	}
	return nil
}

func (b *K8sEndpointBackend) Port() int {
	return -1
}

// Mimics github.com/prometheus/alertmanager/config/config.go's Config struct
// but we can't due to mismatched github.com/prometheus/common versions
type ConfigMapData struct {
	Global       *GlobalConfig      `yaml:"global,omitempty" json:"global,omitempty"`
	Route        *cfg.Route         `yaml:"route,omitempty" json:"route,omitempty"`
	InhibitRules []*cfg.InhibitRule `yaml:"inhibit_rules,omitempty" json:"inhibit_rules,omitempty"`
	Receivers    []*cfg.Receiver    `yaml:"receivers,omitempty" json:"receivers,omitempty"`
	Templates    []string           `yaml:"templates" json:"templates"`
}

func (c *ConfigMapData) Parse(data string) error {
	return yaml.Unmarshal([]byte(data), c)
}

func NewConfigMapDataFrom(data string) (*ConfigMapData, error) {
	c := &ConfigMapData{}
	err := c.Parse(data)
	return c, err
}

func (c *ConfigMapData) Marshal() ([]byte, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	return data, nil
}

type AlertManagerAPI struct {
	Endpoint string
	Api      string
	Route    string
	Verb     string
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

func (a *AlertManagerAPI) IsReady() bool {
	return false
}

func (a *AlertManagerAPI) IsHealthy() bool {
	return false
}

// WithHttpV2
// ## OpenAPI reference
// https://github.com/prometheus/alertmanager/blob/main/api/v2/openapi.yaml
func (a *AlertManagerAPI) WithHttpV2() *AlertManagerAPI {
	a.Api = v2
	return a
}

// WithHttpV1
// ## Reference
// https://prometheus.io/docs/alerting/latest/clients/
func (a *AlertManagerAPI) WithHttpV1() *AlertManagerAPI {
	a.Api = v1
	return a
}

func PostAlert(ctx context.Context, endpoint string, alerts []*PostableAlert) (*http.Response, error) {
	for _, alert := range alerts {
		if err := alert.Must(); err != nil {
			panic(err)
		}
	}
	hclient := &http.Client{}
	reqUrl := (&AlertManagerAPI{
		Endpoint: endpoint,
		Route:    "/alerts",
		Verb:     POST,
	}).WithHttpV2().ConstructHTTP()
	b, err := json.Marshal(alerts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, POST, reqUrl, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Accept", "application/json")
	resp, err := hclient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func PostSilence(ctx context.Context, endpoint string, silence *PostableSilence) (*http.Response, error) {
	if err := silence.Must(); err != nil {
		panic(err)
	}
	hclient := &http.Client{}
	reqUrl := (&AlertManagerAPI{
		Endpoint: endpoint,
		Route:    "/silences",
		Verb:     POST,
	}).WithHttpV2().ConstructHTTP()
	b, err := json.Marshal(silence)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, POST, reqUrl, bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	resp, err := hclient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func DeleteSilence(ctx context.Context, endpoint string, silence *DeletableSilence) (*http.Response, error) {
	if err := silence.Must(); err != nil {
		return nil, shared.WithInternalServerErrorf("%s", err)
	}
	hclient := &http.Client{}
	reqUrl := (&AlertManagerAPI{
		Endpoint: endpoint,
		Route:    "/silences/" + silence.silenceId,
		Verb:     DELETE,
	}).WithHttpV2().ConstructHTTP()
	req, err := http.NewRequestWithContext(ctx, DELETE, reqUrl, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hclient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ValidateIncomingConfig
// Adapted from https://github.com/prometheus/alertmanager/blob/c732372d7d3be49198398d34753080459f01749e/cli/check_config.go#L51
func ValidateIncomingConfig(fileContent string, lg *zap.SugaredLogger) error {
	config, err := cfg.Load(fileContent)
	if err != nil {
		return err
	}
	if config != nil {
		if config.Global != nil {
			lg.Debug("Global config found")
		}
		if config.Route != nil {
			lg.Debug("Route config found")
		}
		lg.Debug(fmt.Sprintf(" - %d inhibit rules", len(config.InhibitRules)))
		lg.Debug(fmt.Sprintf(" - %d receivers", len(config.Receivers)))
		lg.Debug(fmt.Sprintf(" - %d templates", len(config.Templates)))

		if len(config.Templates) > 0 {
			_, err = template.FromGlobs(config.Templates...)
			if err != nil {
				lg.Error(fmt.Sprintf("failed to glob template files with %s for content : %s", err, fileContent))
				return err
			}
		}
	}
	return nil
}

// ReconcileInvalidState : tries to fix detected errors in Alertmanager
func ReconcileInvalidState(config *ConfigMapData, incoming error) error {
	if incoming == nil {
		return nil
	}
	switch msg := incoming.Error(); {
	case msg == NoSmartHostSet:
		config.SetDefaultSMTPServer()
	case FieldNotFound.MatchString(msg):
		panic(fmt.Sprintf("Likely mismatched versions of prometheus/common : %s", msg))
	default:
		return incoming
	}
	return nil
}

func ReconcileInvalidStateLoop(timeout time.Duration, config *ConfigMapData, lg *zap.SugaredLogger) error {
	timeoutTicker := time.NewTicker(timeout)
	var lastSetError error
	for {
		select {
		case <-timeoutTicker.C:
			if lastSetError != nil {
				lastSetError = fmt.Errorf(
					"timeout(%s) when reconciling new alert configs : %s", timeout, lastSetError)
			}
			return lastSetError
		default:
			rawConfig, marshalErr := config.Marshal()
			if marshalErr != nil {
				lastSetError = marshalErr
				continue
			}
			reconcileError := ValidateIncomingConfig(string(rawConfig), lg)
			if reconcileError == nil {
				return nil // success
			}
			err := ReconcileInvalidState(config, reconcileError)
			if err != nil {
				lastSetError = err
				continue
			} // can't return nil after this as there may be a chain of errors to handle
		}
	}
}

// required due to https://github.com/rancher/opni/issues/542
type GlobalConfig struct {
	// ResolveTimeout is the time after which an alert is declared resolved
	// if it has not been updated.
	ResolveTimeout model.Duration `yaml:"resolve_timeout" json:"resolve_timeout"`

	HTTPConfig *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	SMTPFrom           string         `yaml:"smtp_from,omitempty" json:"smtp_from,omitempty"`
	SMTPHello          string         `yaml:"smtp_hello,omitempty" json:"smtp_hello,omitempty"`
	SMTPSmarthost      cfg.HostPort   `yaml:"smtp_smarthost,omitempty" json:"smtp_smarthost,omitempty"`
	SMTPAuthUsername   string         `yaml:"smtp_auth_username,omitempty" json:"smtp_auth_username,omitempty"`
	SMTPAuthPassword   cfg.Secret     `yaml:"smtp_auth_password,omitempty" json:"smtp_auth_password,omitempty"`
	SMTPAuthSecret     cfg.Secret     `yaml:"smtp_auth_secret,omitempty" json:"smtp_auth_secret,omitempty"`
	SMTPAuthIdentity   string         `yaml:"smtp_auth_identity,omitempty" json:"smtp_auth_identity,omitempty"`
	SMTPRequireTLS     bool           `yaml:"smtp_require_tls" json:"smtp_require_tls,omitempty"`
	SlackAPIURL        *cfg.SecretURL `yaml:"slack_api_url,omitempty" json:"slack_api_url,omitempty"`
	SlackAPIURLFile    string         `yaml:"slack_api_url_file,omitempty" json:"slack_api_url_file,omitempty"`
	PagerdutyURL       *cfg.URL       `yaml:"pagerduty_url,omitempty" json:"pagerduty_url,omitempty"`
	OpsGenieAPIURL     *cfg.URL       `yaml:"opsgenie_api_url,omitempty" json:"opsgenie_api_url,omitempty"`
	OpsGenieAPIKey     cfg.Secret     `yaml:"opsgenie_api_key,omitempty" json:"opsgenie_api_key,omitempty"`
	OpsGenieAPIKeyFile string         `yaml:"opsgenie_api_key_file,omitempty" json:"opsgenie_api_key_file,omitempty"`
	WeChatAPIURL       *cfg.URL       `yaml:"wechat_api_url,omitempty" json:"wechat_api_url,omitempty"`
	WeChatAPISecret    cfg.Secret     `yaml:"wechat_api_secret,omitempty" json:"wechat_api_secret,omitempty"`
	WeChatAPICorpID    string         `yaml:"wechat_api_corp_id,omitempty" json:"wechat_api_corp_id,omitempty"`
	VictorOpsAPIURL    *cfg.URL       `yaml:"victorops_api_url,omitempty" json:"victorops_api_url,omitempty"`
	VictorOpsAPIKey    cfg.Secret     `yaml:"victorops_api_key,omitempty" json:"victorops_api_key,omitempty"`
	TelegramAPIUrl     *cfg.URL       `yaml:"telegram_api_url,omitempty" json:"telegram_api_url,omitempty"`
}

// required due to https://github.com/rancher/opni/issues/542
type HTTPClientConfig struct {
	// The HTTP basic authentication credentials for the targets.
	BasicAuth *commoncfg.BasicAuth `yaml:"basic_auth,omitempty" json:"basic_auth,omitempty"`
	// The HTTP authorization credentials for the targets.
	Authorization *commoncfg.Authorization `yaml:"authorization,omitempty" json:"authorization,omitempty"`
	// The OAuth2 client credentials used to fetch a token for the targets.
	OAuth2 *commoncfg.OAuth2 `yaml:"oauth2,omitempty" json:"oauth2,omitempty"`
	// The bearer token for the targets. Deprecated in favour of
	// Authorization.Credentials.
	BearerToken commoncfg.Secret `yaml:"bearer_token,omitempty" json:"bearer_token,omitempty"`
	// The bearer token file for the targets. Deprecated in favour of
	// Authorization.CredentialsFile.
	BearerTokenFile string `yaml:"bearer_token_file,omitempty" json:"bearer_token_file,omitempty"`
	// HTTP proxy server to use to connect to the targets.
	ProxyURL commoncfg.URL `yaml:"proxy_url,omitempty" json:"proxy_url,omitempty"`
	// TLSConfig to use to connect to the targets.
	TLSConfig commoncfg.TLSConfig `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
	// FollowRedirects specifies whether the client should follow HTTP 3xx redirects.
	// The omitempty flag is not set, because it would be hidden from the
	// marshalled configuration when set to false.
	FollowRedirects bool `yaml:"follow_redirects" json:"follow_redirects"`
}
