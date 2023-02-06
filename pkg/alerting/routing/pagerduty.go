package routing

import (
	"fmt"
	"strings"

	cfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

var _ OpniConfig = (*PagerdutyConfig)(nil)

var (
	// DefaultPagerdutyDetails defines the default values for PagerDuty details.
	DefaultPagerdutyDetails = map[string]string{
		"firing":       `{{ template "pagerduty.default.instances" .Alerts.Firing }}`,
		"resolved":     `{{ template "pagerduty.default.instances" .Alerts.Resolved }}`,
		"num_firing":   `{{ .Alerts.Firing | len }}`,
		"num_resolved": `{{ .Alerts.Resolved | len }}`,
	}
)

// PagerdutyConfig configures notifications via PagerDuty.
type PagerdutyConfig struct {
	*cfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	// Change from secret to string since the string is stored in a kube secret anyways
	ServiceKey string `yaml:"service_key,omitempty" json:"service_key,omitempty"`
	// Change from secret to string since the string is stored in a kube secret anyways
	RoutingKey  string                `yaml:"routing_key,omitempty" json:"routing_key,omitempty"`
	URL         *cfg.URL              `yaml:"url,omitempty" json:"url,omitempty"`
	Client      string                `yaml:"client,omitempty" json:"client,omitempty"`
	ClientURL   string                `yaml:"client_url,omitempty" json:"client_url,omitempty"`
	Description string                `yaml:"description,omitempty" json:"description,omitempty"`
	Details     map[string]string     `yaml:"details,omitempty" json:"details,omitempty"`
	Images      []*cfg.PagerdutyImage `yaml:"images,omitempty" json:"images,omitempty"`
	Links       []*cfg.PagerdutyLink  `yaml:"links,omitempty" json:"links,omitempty"`
	Severity    string                `yaml:"severity,omitempty" json:"severity,omitempty"`
	Class       string                `yaml:"class,omitempty" json:"class,omitempty"`
	Component   string                `yaml:"component,omitempty" json:"component,omitempty"`
	Group       string                `yaml:"group,omitempty" json:"group,omitempty"`
}

// AlertManager Compatible unmarshalling that implements the the yaml.Unmarshaler interface.
func (c *PagerdutyConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	DefaultPagerduty := c.Default().(*PagerdutyConfig)
	*c = *DefaultPagerduty
	type plain PagerdutyConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.RoutingKey == "" && c.ServiceKey == "" {
		return fmt.Errorf("missing service or routing key in PagerDuty config")
	}
	if c.Details == nil {
		c.Details = make(map[string]string)
	}
	for k, v := range DefaultPagerdutyDetails {
		if _, ok := c.Details[k]; !ok {
			c.Details[k] = v
		}
	}
	return nil
}

func (c *PagerdutyConfig) Equal(other *PagerdutyConfig) (bool, string) {
	return pagerDutyConfigsAreEqual(c, other)
}

func (c *PagerdutyConfig) ExtractDetails() *alertingv1.EndpointImplementation {
	strArr := strings.Split(c.Description, "\n")
	title := strArr[0]
	body := strings.Join(strArr[1:], "\n")
	return &alertingv1.EndpointImplementation{
		Title:        title,
		Body:         body,
		SendResolved: &c.VSendResolved,
	}
}

func (c *PagerdutyConfig) Default() OpniConfig {
	return &PagerdutyConfig{
		NotifierConfig: &cfg.NotifierConfig{
			VSendResolved: true,
		},
		Description: `{{ template "pagerduty.default.description" .}}`,
		Client:      `{{ template "pagerduty.default.client" . }}`,
		ClientURL:   `{{ template "pagerduty.default.clientURL" . }}`,
	}
}

func (c *PagerdutyConfig) InternalId() string {
	return "pagerduty"
}

func pagerDutyConfigsAreEqual(p1, p2 *PagerdutyConfig) (equal bool, reason string) {
	if p1.RoutingKey != p2.RoutingKey {
		return false, fmt.Sprintf("routing key mismatch %s <-> %s ", p1.RoutingKey, p2.RoutingKey)
	}
	if p1.ServiceKey != p2.ServiceKey {
		return false, fmt.Sprintf("service key mismatch %s <-> %s ", p1.ServiceKey, p2.ServiceKey)
	}
	if p1.URL != p2.URL {
		return false, fmt.Sprintf("url mismatch %s <-> %s ", p1.URL, p2.URL)
	}
	if p1.Client != p2.Client {
		return false, fmt.Sprintf("client mismatch %s <-> %s ", p1.Client, p2.Client)
	}
	if p1.ClientURL != p2.ClientURL {
		return false, fmt.Sprintf("client url mismatch %s <-> %s ", p1.ClientURL, p2.ClientURL)
	}
	if p1.Description != p2.Description {
		return false, fmt.Sprintf("description mismatch %s <-> %s ", p1.Description, p2.Description)
	}
	return true, ""
}
