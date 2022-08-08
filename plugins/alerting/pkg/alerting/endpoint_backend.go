package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"

	cfg "github.com/prometheus/alertmanager/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const LocalBackendEnvToggle = "OPNI_ALERTING_BACKEND_LOCAL"
const LocalAlertManagerPath = "/tmp/alertmanager.yaml"

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
			FieldManager: "kubectl-rollout",
		},
	)
	return err
}

type RuntimeEndpointBackend interface {
	Fetch(ctx context.Context, p *Plugin, key string) (string, error)
	Put(ctx context.Context, p *Plugin, key string, data string) error
	Reload(ctx context.Context, p *Plugin) error
}

// LocalEndpointBackend implements alerting.RuntimeEndpointBackend
type LocalEndpointBackend struct {
	configFilePath string
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

type AlertManagerAPI struct {
	Endpoint string
	Api      string
	Route    string
	Verb     string
}

func (a *AlertManagerAPI) Construct() string {
	return path.Join(a.Endpoint, a.Api, a.Route)
}

func (a *AlertManagerAPI) ConstructHTTP() string {
	return "http://" + a.Construct()
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
