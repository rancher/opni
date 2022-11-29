package routing

import (
	"fmt"

	cfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

var _ OpniConfig = (*SlackConfig)(nil)

type SlackConfig struct {
	cfg.NotifierConfig `yaml:",inline" json:",inline"`

	HTTPConfig *commoncfg.HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	// string since the string is stored in a kube secret anyways
	APIURL     string `yaml:"api_url,omitempty" json:"api_url,omitempty"`
	APIURLFile string `yaml:"api_url_file,omitempty" json:"api_url_file,omitempty"`

	// Slack channel override, (like #other-channel or @username).
	Channel  string `yaml:"channel,omitempty" json:"channel,omitempty"`
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Color    string `yaml:"color,omitempty" json:"color,omitempty"`

	Title       string             `yaml:"title,omitempty" json:"title,omitempty"`
	TitleLink   string             `yaml:"title_link,omitempty" json:"title_link,omitempty"`
	Pretext     string             `yaml:"pretext,omitempty" json:"pretext,omitempty"`
	Text        string             `yaml:"text,omitempty" json:"text,omitempty"`
	Fields      []*cfg.SlackField  `yaml:"fields,omitempty" json:"fields,omitempty"`
	ShortFields bool               `yaml:"short_fields" json:"short_fields,omitempty"`
	Footer      string             `yaml:"footer,omitempty" json:"footer,omitempty"`
	Fallback    string             `yaml:"fallback,omitempty" json:"fallback,omitempty"`
	CallbackID  string             `yaml:"callback_id,omitempty" json:"callback_id,omitempty"`
	IconEmoji   string             `yaml:"icon_emoji,omitempty" json:"icon_emoji,omitempty"`
	IconURL     string             `yaml:"icon_url,omitempty" json:"icon_url,omitempty"`
	ImageURL    string             `yaml:"image_url,omitempty" json:"image_url,omitempty"`
	ThumbURL    string             `yaml:"thumb_url,omitempty" json:"thumb_url,omitempty"`
	LinkNames   bool               `yaml:"link_names" json:"link_names,omitempty"`
	MrkdwnIn    []string           `yaml:"mrkdwn_in,omitempty" json:"mrkdwn_in,omitempty"`
	Actions     []*cfg.SlackAction `yaml:"actions,omitempty" json:"actions,omitempty"`
}

func (c *SlackConfig) Equal(other *SlackConfig) (bool, string) {
	return slackConfigsAreEqual(c, other)
}

func (c *SlackConfig) InternalId() string {
	return "slack"
}

func (c *SlackConfig) ExtractDetails() *alertingv1.EndpointImplementation {
	return &alertingv1.EndpointImplementation{
		Title:        c.Title,
		Body:         c.Text,
		SendResolved: &c.VSendResolved,
	}
}

func (c *SlackConfig) Default() OpniConfig {
	return &SlackConfig{
		NotifierConfig: cfg.NotifierConfig{
			VSendResolved: false,
		},
		Color:      `{{ if eq .Status "firing" }}danger{{ else }}good{{ end }}`,
		Username:   `{{ template "slack.default.username" . }}`,
		Title:      `{{ template "slack.default.title" . }}`,
		TitleLink:  `{{ template "slack.default.titlelink" . }}`,
		IconEmoji:  `{{ template "slack.default.iconemoji" . }}`,
		IconURL:    `{{ template "slack.default.iconurl" . }}`,
		Pretext:    `{{ template "slack.default.pretext" . }}`,
		Text:       `{{ template "slack.default.text" . }}`,
		Fallback:   `{{ template "slack.default.fallback" . }}`,
		CallbackID: `{{ template "slack.default.callbackid" . }}`,
		Footer:     `{{ template "slack.default.footer" . }}`,
	}
}

// AlertManager Compatible unmarshalling that implements the the yaml.Unmarshaler interface.
func (c *SlackConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	DefaultSlack := c.Default().(*SlackConfig)
	*c = *DefaultSlack
	type plain SlackConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.APIURL != "" && len(c.APIURLFile) > 0 {
		return fmt.Errorf("at most one of api_url & api_url_file must be configured")
	}

	return nil
}

func slackConfigsAreEqual(s1, s2 *SlackConfig) (equal bool, reason string) {
	if s1.Channel != s2.Channel {
		return false, fmt.Sprintf("channel mismatch  %s <-> %s ", s1.Channel, s2.Channel)
	}
	if s1.APIURL != s2.APIURL {
		return false, fmt.Sprintf("api url mismatch  %s <-> %s ", s1.APIURL, s2.APIURL)
	}
	return true, ""
}
