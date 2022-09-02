package alerting

import (
	"fmt"
	"net/url"
	"time"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/common/model"
)

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
func mustParseURL(s string) *config.URL {
	u, err := parseURL(s)
	if err != nil {
		panic(err)
	}
	return u
}
func parseURL(s string) (*config.URL, error) {
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
	return &config.URL{URL: u}, nil
}

func (c *ConfigMapData) SetDefaultSMTPServer() {
	defConfig := DefaultGlobalConfig()
	c.Global = &defConfig
	c.Global.SMTPSmarthost = config.HostPort{Port: fmt.Sprintf("%d", DefaultSMTPServerPort)}

}

func (c *ConfigMapData) UnsetSMTPServer() {
	c.Global.SMTPRequireTLS = false
	c.Global.SMTPFrom = ""
	c.Global.SMTPSmarthost = config.HostPort{}
	c.Global.SMTPHello = ""
	c.Global.SMTPAuthUsername = ""
	c.Global.SMTPAuthPassword = ""
	c.Global.SMTPAuthSecret = config.Secret("")
}
