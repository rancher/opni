package alerting

import (
	"net/mail"
	"net/url"
	"path"
	"strings"

	cfg "github.com/prometheus/alertmanager/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/validation"
	"gopkg.in/yaml.v2"
)

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

type ConfigMapData struct {
	Route        cfg.Route         `yaml:"route"`
	Receivers    []*cfg.Receiver   `yaml:"receivers"`
	InhibitRules []cfg.InhibitRule `yaml:"inhibit_rules"`
}

func (c *ConfigMapData) Parse(data string) error {
	return yaml.Unmarshal([]byte(data), c)
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

func NewSlackReceiver(endpoint *alertingv1alpha.SlackEndpoint) (*cfg.Receiver, error) {
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
		Name: endpoint.Name,
		SlackConfigs: []*cfg.SlackConfig{
			{
				APIURL:  &cfg.SecretURL{URL: parsedURL},
				Channel: channel,
			},
		},
	}, nil
}

func NewMailReceiver(endpoint *alertingv1alpha.EmailEndpoint) (*cfg.Receiver, error) {
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
		Name: endpoint.Name,
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
