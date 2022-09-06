package alerting

import (
	"fmt"
	cfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"time"
)

var DefaultSlackConfig = SlackConfig{
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

// Receiver configuration provides configuration on how to contact a receiver.
// Required to overwrite certain fields in alertmanager's Receiver : https://github.com/rancher/opni/issues/544
type Receiver struct {
	// A unique identifier for this receiver.
	Name string `yaml:"name" json:"name"`

	EmailConfigs     []*cfg.EmailConfig     `yaml:"email_configs,omitempty" json:"email_configs,omitempty"`
	PagerdutyConfigs []*cfg.PagerdutyConfig `yaml:"pagerduty_configs,omitempty" json:"pagerduty_configs,omitempty"`
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

// PostableAlert : corresponds to the data AlertManager API
// needs to trigger alerts
type PostableAlert struct {
	StartsAt     *time.Time         `json:"startsAt,omitempty"`
	EndsAt       *time.Time         `json:"endsAt,omitempty"`
	Annotations  *map[string]string `json:"annotations,omitempty"`
	Labels       map[string]string  `json:"labels"`
	GeneratorURL *string            `json:"generatorURL,omitempty"`
}

// WithCondition In our basic model each receiver is uniquely
// identified by its name, which in AlertManager can be routed
// to by the label `alertname`.
func (p *PostableAlert) WithCondition(conditionId string) {
	if p.Labels == nil {
		p.Labels = make(map[string]string)
	}
	p.Labels["alertname"] = conditionId
	p.Labels["conditionId"] = conditionId
}

// WithRuntimeInfo adds the runtime information to the alert.
func (p *PostableAlert) WithRuntimeInfo(key string, value string) {
	if p.Annotations == nil {
		newMap := map[string]string{}
		p.Annotations = &newMap
	}
	(*p.Annotations)[key] = value
}

func (p *PostableAlert) Must() error {
	if p.Labels == nil {
		return fmt.Errorf("missting PostableAlert.Labels")
	}
	if v, ok := p.Labels["alertname"]; !ok || v == "" {
		return fmt.Errorf(`missting PostableAlert.Labels["alertname"]`)
	}
	return nil
}

type Matcher struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	IsRegex bool   `json:"isRegex"`
	IsEqual *bool  `json:"isEqual,omitempty"`
}

// DeletableSilence fills in api path `"/silence/{silenceID}"`
type DeletableSilence struct {
	silenceId string
}

func (d *DeletableSilence) WithSilenceId(silenceId string) {
	d.silenceId = silenceId
}

func (d *DeletableSilence) Must() error {
	if d.silenceId == "" {
		return fmt.Errorf("missing silenceId")
	}
	return nil
}

// PostableSilence struct for PostableSilence
type PostableSilence struct {
	Id        *string   `json:"id,omitempty"`
	Matchers  []Matcher `json:"matchers"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	CreatedBy string    `json:"createdBy"`
	Comment   string    `json:"comment"`
}

type PostSilencesResponse struct {
	SilenceID *string `json:"silenceID,omitempty"`
}

func (p *PostSilencesResponse) GetSilenceId() string {
	if p == nil || p.SilenceID == nil {
		return ""
	}
	return *p.SilenceID
}

// WithCondition In our basic model each receiver is uniquely
// identified by its name, which in AlertManager can be routed
// to by the label `alertname`.
func (p *PostableSilence) WithCondition(conditionId string) {
	if p.Matchers == nil {
		p.Matchers = make([]Matcher, 0)
	}
	p.Matchers = append(p.Matchers, Matcher{Name: conditionId})
}

func (p *PostableSilence) WithDuration(dur time.Duration) {
	start := time.Now()
	end := start.Add(dur)
	p.StartsAt = start
	p.EndsAt = end
}

func (p *PostableSilence) WithSilenceId(silenceId string) {
	p.Id = &silenceId
}

func (p *PostableSilence) Must() error {
	if p.Matchers == nil {
		return fmt.Errorf("missing PostableSilence.Matchers")
	}
	if len(p.Matchers) == 0 {
		return fmt.Errorf("missing PostableSilence.Matchers")
	}
	if p.StartsAt.IsZero() {
		return fmt.Errorf("missing PostableSilence.StartsAt")
	}
	if p.EndsAt.IsZero() {
		return fmt.Errorf("missing PostableSilence.EndsAt")
	}
	return nil
}
