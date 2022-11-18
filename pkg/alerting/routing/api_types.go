package routing

import (
	"fmt"
	"net/url"
	"time"

	"github.com/containerd/containerd/pkg/cri/config"
	cfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	DefaultSlackConfig = SlackConfig{
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
	// DefaultEmailConfig defines default values for Email configurations.
	DefaultEmailConfig = EmailConfig{
		NotifierConfig: cfg.NotifierConfig{
			VSendResolved: false,
		},
		HTML: `{{ template "email.default.html" . }}`,
		Text: ``,
	}
	normalizeTitle = cases.Title(language.AmericanEnglish)

	// DefaultPagerdutyConfig defines default values for PagerDuty configurations.
	DefaultPagerdutyConfig = PagerdutyConfig{
		NotifierConfig: &cfg.NotifierConfig{
			VSendResolved: true,
		},
		Description: `{{ template "pagerduty.default.description" .}}`,
		Client:      `{{ template "pagerduty.default.client" . }}`,
		ClientURL:   `{{ template "pagerduty.default.clientURL" . }}`,
	}
	// DefaultPagerdutyDetails defines the default values for PagerDuty details.
	DefaultPagerdutyDetails = map[string]string{
		"firing":       `{{ template "pagerduty.default.instances" .Alerts.Firing }}`,
		"resolved":     `{{ template "pagerduty.default.instances" .Alerts.Resolved }}`,
		"num_firing":   `{{ .Alerts.Firing | len }}`,
		"num_resolved": `{{ .Alerts.Resolved | len }}`,
	}
)

// Receiver configuration provides configuration on how to contact a receiver.
// Required to overwrite certain fields in alertmanager's Receiver : https://github.com/rancher/opni/issues/544
type Receiver struct {
	// A unique identifier for this receiver.
	Name string `yaml:"name" json:"name"`

	EmailConfigs     []*EmailConfig         `yaml:"email_configs,omitempty" json:"email_configs,omitempty"`
	PagerdutyConfigs []*PagerdutyConfig     `yaml:"pagerduty_configs,omitempty" json:"pagerduty_configs,omitempty"`
	SlackConfigs     []*SlackConfig         `yaml:"slack_configs,omitempty" json:"slack_configs,omitempty"`
	WebhookConfigs   []*cfg.WebhookConfig   `yaml:"webhook_configs,omitempty" json:"webhook_configs,omitempty"`
	OpsGenieConfigs  []*cfg.OpsGenieConfig  `yaml:"opsgenie_configs,omitempty" json:"opsgenie_configs,omitempty"`
	WechatConfigs    []*cfg.WechatConfig    `yaml:"wechat_configs,omitempty" json:"wechat_configs,omitempty"`
	PushoverConfigs  []*cfg.PushoverConfig  `yaml:"pushover_configs,omitempty" json:"pushover_configs,omitempty"`
	VictorOpsConfigs []*cfg.VictorOpsConfig `yaml:"victorops_configs,omitempty" json:"victorops_configs,omitempty"`
	SNSConfigs       []*cfg.SNSConfig       `yaml:"sns_configs,omitempty" json:"sns_configs,omitempty"`
	TelegramConfigs  []*cfg.TelegramConfig  `yaml:"telegram_configs,omitempty" json:"telegram_configs,omitempty"`
}

func (c *Receiver) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain Receiver
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Name == "" {
		return fmt.Errorf("missing name in receiver")
	}
	return nil
}

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

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *EmailConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultEmailConfig
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

func (c *SlackConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSlackConfig
	type plain SlackConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	if c.APIURL != "" && len(c.APIURLFile) > 0 {
		return fmt.Errorf("at most one of api_url & api_url_file must be configured")
	}

	return nil
}

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

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PagerdutyConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultPagerdutyConfig
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

// required due to https://github.com/rancher/opni/issues/542
type GlobalConfig struct {
	// ResolveTimeout is the time after which an alert is declared resolved
	// if it has not been updated.
	ResolveTimeout model.Duration `yaml:"resolve_timeout" json:"resolve_timeout"`

	HTTPConfig *HTTPClientConfig `yaml:"http_config,omitempty" json:"http_config,omitempty"`

	SMTPFrom         string       `yaml:"smtp_from,omitempty" json:"smtp_from,omitempty"`
	SMTPHello        string       `yaml:"smtp_hello,omitempty" json:"smtp_hello,omitempty"`
	SMTPSmarthost    cfg.HostPort `yaml:"smtp_smarthost,omitempty" json:"smtp_smarthost,omitempty"`
	SMTPAuthUsername string       `yaml:"smtp_auth_username,omitempty" json:"smtp_auth_username,omitempty"`
	// Changed from Secret to string to avoid issues with the yaml parser
	SMTPAuthPassword string `yaml:"smtp_auth_password,omitempty" json:"smtp_auth_password,omitempty"`
	// Changed from Secret to string to avoid issues with the yaml parser
	SMTPAuthSecret     string         `yaml:"smtp_auth_secret,omitempty" json:"smtp_auth_secret,omitempty"`
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

const DefaultSMTPServerPort = 25
const DefaultSMTPServerHost = "localhost"

var defaultHTTPConfig = &HTTPClientConfig{
	FollowRedirects: true,
}

func DefaultGlobalConfig() GlobalConfig {
	return GlobalConfig{
		ResolveTimeout: model.Duration(5 * time.Minute),
		HTTPConfig:     defaultHTTPConfig,

		SMTPHello:       "localhost",
		SMTPRequireTLS:  true,
		PagerdutyURL:    mustParseURL("https://events.pagerduty.com/v2/enqueue"),
		OpsGenieAPIURL:  mustParseURL("https://api.opsgenie.com/"),
		WeChatAPIURL:    mustParseURL("https://qyapi.weixin.qq.com/cgi-bin/"),
		VictorOpsAPIURL: mustParseURL("https://alert.victorops.com/integrations/generic/20131114/alert/"),
		TelegramAPIUrl:  mustParseURL("https://api.telegram.org"),
	}
}
func mustParseURL(s string) *cfg.URL {
	u, err := parseURL(s)
	if err != nil {
		panic(err)
	}
	return u
}
func parseURL(s string) (*cfg.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme %q for URL", u.Scheme)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("missing host for URL")
	}
	return &cfg.URL{URL: u}, nil
}

func (r *RoutingTree) SetDefaultSMTPServer() {
	defConfig := DefaultGlobalConfig()
	r.Global = &defConfig
	r.Global.SMTPSmarthost = cfg.HostPort{Port: fmt.Sprintf("%d", DefaultSMTPServerPort)}
}

func (r *RoutingTree) SetDefaultSMTPFrom() {
	r.Global.SMTPFrom = "alerting@opni.io"
}

func (r *RoutingTree) UnsetSMTPServer() {
	r.Global.SMTPRequireTLS = false
	r.Global.SMTPFrom = ""
	r.Global.SMTPSmarthost = cfg.HostPort{}
	r.Global.SMTPHello = ""
	r.Global.SMTPAuthUsername = ""
	r.Global.SMTPAuthPassword = ""
	r.Global.SMTPAuthSecret = ""
}
