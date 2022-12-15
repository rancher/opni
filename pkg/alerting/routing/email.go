package routing

import (
	"fmt"

	"github.com/containerd/containerd/pkg/cri/config"
	cfg "github.com/prometheus/alertmanager/config"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

var _ OpniConfig = (*EmailConfig)(nil)

type EmailConfig struct {
	cfg.NotifierConfig `yaml:",inline" json:",inline"`
	To                 string       `yaml:"to,omitempty" json:"to,omitempty"`
	From               string       `yaml:"from,omitempty" json:"from,omitempty"`
	Hello              string       `yaml:"hello,omitempty" json:"hello,omitempty"`
	Smarthost          cfg.HostPort `yaml:"smarthost,omitempty" json:"smarthost,omitempty"`
	AuthUsername       string       `yaml:"auth_username,omitempty" json:"auth_username,omitempty"`
	// Change from secret to string since the string is stored in a kube secret anyways
	AuthPassword string `yaml:"auth_password,omitempty" json:"auth_password,omitempty"`
	// Change from secret to string since the string is stored in a kube secret anyways
	AuthSecret   string            `yaml:"auth_secret,omitempty" json:"auth_secret,omitempty"`
	AuthIdentity string            `yaml:"auth_identity,omitempty" json:"auth_identity,omitempty"`
	Headers      map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	HTML         string            `yaml:"html,omitempty" json:"html,omitempty"`
	Text         string            `yaml:"text,omitempty" json:"text,omitempty"`
	RequireTLS   *bool             `yaml:"require_tls,omitempty" json:"require_tls,omitempty"`
	TLSConfig    config.TLSConfig  `yaml:"tls_config,omitempty" json:"tls_config,omitempty"`
}

func (c *EmailConfig) Equal(other *EmailConfig) (bool, string) {
	return emailConfigsAreEqual(c, other)
}

func (c *EmailConfig) InternalId() string {
	return "email"
}

func (c *EmailConfig) ExtractDetails() *alertingv1.EndpointImplementation {
	return &alertingv1.EndpointImplementation{
		Title:        c.Headers["Subject"],
		Body:         c.HTML,
		SendResolved: &c.VSendResolved,
	}
}

func (c *EmailConfig) Default() OpniConfig {
	return &EmailConfig{
		NotifierConfig: cfg.NotifierConfig{
			VSendResolved: false,
		},
		HTML: `{{ template "email.default.html" . }}`,
		Text: ``,
	}
}

// AlertManager Compatible unmarshalling that implements the the yaml.Unmarshaler interface.
func (c *EmailConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	DefaultEmail := c.Default().(*EmailConfig)
	*c = *DefaultEmail
	type plain EmailConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.To == "" {
		return fmt.Errorf("missing to address in email config")
	}
	// Header names are case-insensitive, check for collisions.
	normalizedHeaders := map[string]string{}
	for h, v := range c.Headers {
		normalized := normalizeTitle.String(h)
		if _, ok := normalizedHeaders[normalized]; ok {
			return fmt.Errorf("duplicate header %q in email config", normalized)
		}
		normalizedHeaders[normalized] = v
	}
	c.Headers = normalizedHeaders

	return nil
}

func emailConfigsAreEqual(e1, e2 *EmailConfig) (equal bool, reason string) {
	if e1.To != e2.To {
		return false, fmt.Sprintf("to mismatch %s <-> %s", e1.To, e2.To)
	}
	if e1.From != e2.From {
		return false, fmt.Sprintf("from mismatch %s <-> %s ", e1.From, e2.From)
	}
	if e1.Smarthost != e2.Smarthost {
		return false, fmt.Sprintf("smarthost mismatch %s <-> %s ", e1.Smarthost, e2.Smarthost)
	}
	if e1.AuthUsername != e2.AuthUsername {
		return false, fmt.Sprintf("auth username mismatch %s <-> %s ", e1.AuthUsername, e2.AuthUsername)
	}
	if e1.AuthPassword != e2.AuthPassword {
		return false, fmt.Sprintf("auth password mismatch %s <-> %s ", e1.AuthPassword, e2.AuthPassword)
	}
	if e1.AuthSecret != e2.AuthSecret {
		return false, fmt.Sprintf("auth secret mismatch %s <-> %s ", e1.AuthSecret, e2.AuthSecret)
	}
	if e1.RequireTLS != e2.RequireTLS {
		return false, fmt.Sprintf("require tls mismatch %v <-> %v ", e1.RequireTLS, e2.RequireTLS)
	}
	if e1.HTML != e2.HTML {
		return false, fmt.Sprintf("html mismatch %s <-> %s ", e1.HTML, e2.HTML)
	}
	if e1.Text != e2.Text {
		return false, fmt.Sprintf("text mismatch %s <-> %s ", e1.Text, e2.Text)
	}
	return true, ""
}
