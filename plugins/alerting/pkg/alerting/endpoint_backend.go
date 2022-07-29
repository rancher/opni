package alerting

import (
	"context"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path"
	"strings"

	cfg "github.com/prometheus/alertmanager/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/validation"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const LocalBackendEnvToggle = "OPNI_ALERTING_BACKEND_LOCAL"

const route = `
route:
  group_by: ['alertname']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 1h
  receiver: 'web.hook'
`

const receivers = `
receivers:
  - name: 'web.hook'
    webhook_configs:
      - url: 'http://127.0.0.1:5001/'
`

const inihibit_rules = `
inhibit_rules:
- source_match:
  severity: 'critical'
  target_match:
  severity: 'warning'
  equal: ['alertname', 'dev', 'instance']`

var DefaultAlertManager = strings.Join([]string{
	strings.TrimSpace(route),
	receivers,
	strings.TrimSpace(inihibit_rules),
}, "\n")

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
			FieldManager: "kubectl-rollour",
		},
	)
	return err
}

type RuntimeEndpointBackend interface {
	Fetch(ctx context.Context, p *Plugin, key string) (string, error)
	Put(ctx context.Context, p *Plugin, key string, data string) error
	Reload(ctx context.Context, p *Plugin) error
}

// implements alerting.RuntimeEndpointBackend
type LocalEndpointBackend struct {
	env            *test.Environment
	configFilePath string
}

func NewLocalEndpointBackend(env *test.Environment) {
	env = &test.Environment{
		TestBin: "../../../../testbin/bin",
	}
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
	ctx context.Context, p *Plugin, key string, data string) error {
	err := os.WriteFile(b.configFilePath, []byte(data), 0644)
	return err
}

func (b *LocalEndpointBackend) Reload(ctx context.Context,
	p *Plugin) error {
	return nil
}

type K8sEndpointBackend struct {
	config *k8s.Config
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
			fmt.Sprintf("K8s runtime error, config map : %s not found : %s",
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

func (b *K8sEndpointBackend) Put(ctx context.Context, p *Plugin, key string, data string) error {
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
	cfgMap.Data[key] = data

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

type ConfigMapData struct {
	Route        cfg.Route         `yaml:"route"`
	Receivers    []*cfg.Receiver   `yaml:"receivers"`
	InhibitRules []cfg.InhibitRule `yaml:"inhibit_rules"`
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

func (c *ConfigMapData) AppendReceiver(recv *cfg.Receiver) {
	c.Receivers = append(c.Receivers, recv)
}

func (c *ConfigMapData) GetReceivers() []*cfg.Receiver {
	return c.Receivers
}

// Assumptions:
// - Id is unique among receivers
// - Receiver Name corresponds with Ids one-to-one
func (c *ConfigMapData) find(id string) (int, error) {
	foundIdx := -1
	for idx, r := range c.Receivers {
		if r.Name == id {
			foundIdx = idx
			break
		}
	}
	if foundIdx < 0 {
		return foundIdx, fmt.Errorf("receiver with id %s not found in alertmanager backend", id)
	}
	return foundIdx, nil
}

func (c *ConfigMapData) UpdateReceiver(id string, recv *cfg.Receiver) error {
	if recv == nil {
		return fmt.Errorf("Nil receiver passed to UpdateReceiver")
	}
	idx, err := c.find(id)
	if err != nil {
		return err
	}
	c.Receivers[idx] = recv
	return nil
}

func (c *ConfigMapData) DeleteReceiver(id string) error {
	idx, err := c.find(id)
	if err != nil {
		return err
	}
	c.Receivers = slices.Delete(c.Receivers, idx, idx+1)
	return nil
}

func NewSlackReceiver(id string, endpoint *alertingv1alpha.SlackEndpoint) (*cfg.Receiver, error) {
	parsedURL, err := url.Parse(endpoint.ApiUrl)
	if err != nil {
		return nil, err
	}
	channel := strings.TrimSpace(endpoint.Channel)
	if !strings.HasPrefix(channel, "#") {
		//FIXME
		return nil, shared.AlertingErrInvalidSlackChannel
	}

	return &cfg.Receiver{
		Name: id,
		SlackConfigs: []*cfg.SlackConfig{
			{
				APIURL:  &cfg.SecretURL{URL: parsedURL},
				Channel: channel,
			},
		},
	}, nil
}

func NewEmailReceiver(id string, endpoint *alertingv1alpha.EmailEndpoint) (*cfg.Receiver, error) {
	_, err := mail.ParseAddress(endpoint.To)
	if err != nil {
		return nil, validation.Errorf("Invalid Destination email : %w", err)
	}

	if endpoint.From != nil {
		_, err := mail.ParseAddress(*endpoint.From)
		if err != nil {
			return nil, validation.Errorf("Invalid Sender email : %w", err)
		}
	}

	return &cfg.Receiver{
		Name: id,
		EmailConfigs: func() []*cfg.EmailConfig {
			if endpoint.From == nil {
				return []*cfg.EmailConfig{
					{
						To:   endpoint.To,
						From: "alertmanager@localhost",
					},
				}
			}
			return []*cfg.EmailConfig{
				{
					To:   endpoint.To,
					From: *endpoint.From,
				},
			}
		}()}, nil
}

type AlertManagerAPI struct {
	Endpoint string
	Api      string
	Route    string
	Verb     string
}

func (a *AlertManagerAPI) Construct() string {
	return path.Join(a.Endpoint, a.Api, a.Route)
}

func (a *AlertManagerAPI) IsReady() bool {
	return false
}

func (a *AlertManagerAPI) IsHealthy() bool {
	return false
}

// WithHttpV2
//## OpenAPI reference
// https://github.com/prometheus/alertmanager/blob/main/api/v2/openapi.yaml
//
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
