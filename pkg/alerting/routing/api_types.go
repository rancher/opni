package routing

import (
	"fmt"
	"net/url"
	"time"

	cfg "github.com/prometheus/alertmanager/config"
	commoncfg "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var _ OpniReceiver = (*Receiver)(nil)

var (
	normalizeTitle = cases.Title(language.AmericanEnglish)
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

func (r *Receiver) Equal(other *Receiver) (bool, string) {
	return receiversAreEqual(r, other)
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
