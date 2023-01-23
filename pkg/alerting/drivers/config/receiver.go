package config

/*
Contains the specifications of transactions on an AlertManager config.
*/

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/prometheus/common/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"gopkg.in/yaml.v2"
)

const missingBody = "<missing body>"
const missingTitle = "<missing title>"

// Extends the receiver configs of AlertManager, e.g. SlackConfig, EmailConfig...
// panics when the receiver type
type OpniReceiver interface {
	InternalId() string
	// FIXME: maybe this isn't necessary, just pass in all alerts with {{ .Annotations.Body }} {{ .Annotations.Title }}
	// Note this is necessary when updating individual endpoints and propagating those details
	ExtractMessage() *alertingv1.EndpointImplementation
	StoreMessage(details *alertingv1.EndpointImplementation)
	Configure(*alertingv1.AlertEndpoint) OpniReceiver
	Clone() OpniReceiver
	MarshalYAML() ([]byte, error)
	yaml.Unmarshaler
}

func ExtractReceiver(unmarshall func(interface{}) error, data interface{}) (OpniReceiver, error) {
	type slack SlackConfig
	type email EmailConfig
	type pagerduty PagerdutyConfig
	type webhook WebhookConfig
	type opsGenie OpsGenieConfig
	type victorOps VictorOpsConfig
	type wechat WechatConfig
	type pushover PushoverConfig
	type sns SNSConfig
	type telegram TelegramConfig

	slackCfg := &SlackConfig{}
	emailCfg := &EmailConfig{}
	pagerdutyCfg := &PagerdutyConfig{}
	webhookCfg := &WebhookConfig{}
	opsGenieCfg := &OpsGenieConfig{}
	victorOpsCfg := &VictorOpsConfig{}
	wechatCfg := &WechatConfig{}
	pushoverCfg := &PushoverConfig{}
	snsCfg := &SNSConfig{}
	telegramCfg := &TelegramConfig{}
	if err := unmarshall((*slack)(slackCfg)); err == nil {
		return slackCfg, nil
	}
	if err := unmarshall((*email)(emailCfg)); err == nil {
		return emailCfg, nil
	}
	if err := unmarshall((*pagerduty)(pagerdutyCfg)); err == nil {
		return pagerdutyCfg, nil
	}
	if err := unmarshall((*webhook)(webhookCfg)); err == nil {
		return webhookCfg, nil
	}
	if err := unmarshall((*opsGenie)(opsGenieCfg)); err == nil {
		return opsGenieCfg, nil
	}
	if err := unmarshall((*victorOps)(victorOpsCfg)); err == nil {
		return victorOpsCfg, nil
	}
	if err := unmarshall((*wechat)(wechatCfg)); err == nil {
		return wechatCfg, nil
	}
	if err := unmarshall((*pushover)(pushoverCfg)); err == nil {
		return pushoverCfg, nil
	}
	if err := unmarshall((*sns)(snsCfg)); err == nil {
		return snsCfg, nil
	}
	if err := unmarshall((*telegram)(telegramCfg)); err == nil {
		return telegramCfg, nil
	}
	return nil, fmt.Errorf("unknown receiver type")
}

// Takes a collection of OpniReceivers and converts them to a single AlertManager
// this function will panic if it cannot convert the receivers
func BuildReceiver(receiverId string, recvs []OpniReceiver) (*Receiver, error) {
	slackCfg := []*SlackConfig{}
	emailCfg := []*EmailConfig{}
	pagerdutyCfg := []*PagerdutyConfig{}
	webhookCfg := []*WebhookConfig{}
	opsgenieCfg := []*OpsGenieConfig{}
	victoropsCfg := []*VictorOpsConfig{}
	wechatCfg := []*WechatConfig{}
	pushoverCfg := []*PushoverConfig{}
	snsCfg := []*SNSConfig{}
	telegramCfg := []*TelegramConfig{}

	if len(recvs) == 0 {
		return nil, fmt.Errorf("no receivers to build")
	}

	for _, recv := range recvs {
		switch recv.InternalId() {
		case shared.InternalSlackId:
			slackCfg = append(slackCfg, recv.(*SlackConfig))
		case shared.InternalEmailId:
			emailCfg = append(emailCfg, recv.(*EmailConfig))
		case shared.InternalPagerdutyId:
			pagerdutyCfg = append(pagerdutyCfg, recv.(*PagerdutyConfig))
		case shared.InternalWebhookId:
			webhookCfg = append(webhookCfg, recv.(*WebhookConfig))
		case shared.InternalOpsGenieId:
			fallthrough
		case shared.InternalVictorOpsId:
			fallthrough
		case shared.InternalWechatId:
			fallthrough
		case shared.InternalPushoverId:
			fallthrough
		case shared.InternalSNSId:
			fallthrough
		case shared.InternalTelegramId:
			fallthrough
		default:
			return nil, fmt.Errorf("unknown receiver type %s", recv.InternalId())
		}
	}
	if len(slackCfg)+len(emailCfg)+len(pagerdutyCfg)+len(webhookCfg) == 0 {
		return nil, fmt.Errorf("no receivers to configs parsed")
	}

	return &Receiver{
		Name:             receiverId,
		SlackConfigs:     slackCfg,
		EmailConfigs:     emailCfg,
		PagerdutyConfigs: pagerdutyCfg,
		WebhookConfigs:   webhookCfg,
		OpsGenieConfigs:  opsgenieCfg,
		VictorOpsConfigs: victoropsCfg,
		WechatConfigs:    wechatCfg,
		PushoverConfigs:  pushoverCfg,
		SNSConfigs:       snsCfg,
		TelegramConfigs:  telegramCfg,
	}, nil
}

var _ OpniReceiver = (*EmailConfig)(nil)
var _ OpniReceiver = (*SlackConfig)(nil)
var _ OpniReceiver = (*PagerdutyConfig)(nil)
var _ OpniReceiver = (*WebhookConfig)(nil)
var _ OpniReceiver = (*OpsGenieConfig)(nil)
var _ OpniReceiver = (*VictorOpsConfig)(nil)
var _ OpniReceiver = (*WechatConfig)(nil)
var _ OpniReceiver = (*PushoverConfig)(nil)
var _ OpniReceiver = (*SNSConfig)(nil)
var _ OpniReceiver = (*TelegramConfig)(nil)

// var _ OpniReceiver = (*DiscordConfig)(nil)
// var _ OpniReceiver = (*WebexConfig)(nil)

// --- OpniReceiver Implementations

func (c *EmailConfig) InternalId() string {
	return shared.InternalEmailId
}

func (c *EmailConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	res := &alertingv1.EndpointImplementation{}
	if def, ok := c.Headers["Subject"]; ok {
		res.Title = def
	} else {
		res.Title = missingBody
	}
	res.Body = c.HTML
	res.SendResolved = &c.VSendResolved
	return res
}

func (c *EmailConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	if def := details.SendResolved; def != nil {
		c.NotifierConfig = NotifierConfig{
			VSendResolved: *def,
		}
	} else {
		c.NotifierConfig = NotifierConfig{
			VSendResolved: false,
		}
	}
	if c.Headers == nil {
		c.Headers = map[string]string{}
	}
	c.Headers["Subject"] = details.Body
	c.HTML = details.Body
}

func (c *EmailConfig) Configure(endp *alertingv1.AlertEndpoint) OpniReceiver {
	emailSpec := endp.GetEmail()
	c.To = emailSpec.To
	if emailSpec.SmtpFrom != nil {
		c.From = *emailSpec.SmtpFrom
	}
	if emailSpec.SmtpSmartHost != nil {
		arr := strings.Split(*emailSpec.SmtpSmartHost, ":")
		c.Smarthost = HostPort{
			Host: arr[0],
			Port: arr[1],
		}
	}
	if emailSpec.SmtpAuthUsername != nil {
		c.AuthUsername = *emailSpec.SmtpAuthUsername
	}
	if emailSpec.SmtpAuthPassword != nil {
		c.AuthPassword = *emailSpec.SmtpAuthPassword
	}
	if emailSpec.SmtpAuthIdentity != nil {
		c.AuthIdentity = *emailSpec.SmtpAuthIdentity
	}
	return c
}

func (c *EmailConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *EmailConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *SlackConfig) InternalId() string {
	return shared.InternalSlackId
}

func (c *SlackConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	res := &alertingv1.EndpointImplementation{}
	res.Title = c.Title
	res.Body = c.Text
	res.SendResolved = &c.VSendResolved
	return res
}

func (c *SlackConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	if def := details.SendResolved; def != nil {
		c.NotifierConfig = NotifierConfig{
			VSendResolved: *def,
		}
	} else {
		c.NotifierConfig = NotifierConfig{
			VSendResolved: false,
		}
	}
	c.Title = details.Title
	c.Text = details.Body
}

func (c *SlackConfig) Configure(endp *alertingv1.AlertEndpoint) OpniReceiver {
	slackSpec := endp.GetSlack()
	parsedURL, err := url.Parse(slackSpec.WebhookUrl)
	if err != nil {
		panic(err)
	}
	c.APIURL = &config.URL{
		URL: parsedURL,
	}
	c.Channel = slackSpec.Channel
	return c
}

func (c *SlackConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *SlackConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *PagerdutyConfig) InternalId() string {
	return shared.InternalPagerdutyId
}

func (c *PagerdutyConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	res := &alertingv1.EndpointImplementation{}
	if title, ok := c.Details["title"]; ok {
		res.Title = title
	} else {
		res.Title = missingTitle
	}
	res.Body = c.Description
	res.SendResolved = &c.VSendResolved
	return res
}

func (c *PagerdutyConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	if def := details.SendResolved; def != nil {
		c.NotifierConfig = NotifierConfig{
			VSendResolved: *def,
		}
	} else {
		c.NotifierConfig = NotifierConfig{
			VSendResolved: false,
		}
	}
	if c.Details == nil {
		c.Details = map[string]string{}
	}
	c.Details = lo.Assign(
		c.Details,
		DefaultPagerdutyDetails,
		map[string]string{
			"title": details.Title,
		},
	)
	c.Description = details.Body
}

func (c *PagerdutyConfig) Configure(endp *alertingv1.AlertEndpoint) OpniReceiver {
	pagerdutySpec := endp.GetPagerDuty()
	if pagerdutySpec.ServiceKey != "" {
		c.ServiceKey = pagerdutySpec.ServiceKey
	}
	if pagerdutySpec.IntegrationKey != "" {
		c.ServiceKey = pagerdutySpec.IntegrationKey
	}
	return c
}

func (c *PagerdutyConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *PagerdutyConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *WebhookConfig) InternalId() string {
	return shared.InternalWebhookId
}

func (c *WebhookConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	return &alertingv1.EndpointImplementation{}
}

func (c *WebhookConfig) StoreMessage(details *alertingv1.EndpointImplementation) {}

func (c *WebhookConfig) Configure(endp *alertingv1.AlertEndpoint) OpniReceiver {
	webhookSpec := endp.GetWebhook()
	parsedURL := util.Must(url.Parse(webhookSpec.Url))
	c.URL = &config.URL{
		URL: parsedURL,
	}
	return c
}

func (c *WebhookConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *WebhookConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *OpsGenieConfig) InternalId() string {
	return shared.InternalOpsGenieId
}

func (c *OpsGenieConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	//TODO
	return nil
}

func (c *OpsGenieConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	//TODO
}

func (c *OpsGenieConfig) Configure(*alertingv1.AlertEndpoint) OpniReceiver {
	return c
}

func (c *OpsGenieConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *OpsGenieConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *VictorOpsConfig) InternalId() string {
	return shared.InternalVictorOpsId
}

func (c *VictorOpsConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	//TODO
	return nil
}

func (c *VictorOpsConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	//TODO
}

func (c *VictorOpsConfig) Configure(*alertingv1.AlertEndpoint) OpniReceiver {
	return c
}

func (c *VictorOpsConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *VictorOpsConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *WechatConfig) InternalId() string {
	return shared.InternalWechatId
}

func (c *WechatConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	//TODO
	return nil
}

func (c *WechatConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	//TODO
}

func (c *WechatConfig) Configure(*alertingv1.AlertEndpoint) OpniReceiver {
	return c
}

func (c *WechatConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *WechatConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *PushoverConfig) InternalId() string {
	return shared.InternalPushoverId
}

func (c *PushoverConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	//TODO
	return nil
}

func (c *PushoverConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	//TODO
}

func (c *PushoverConfig) Configure(*alertingv1.AlertEndpoint) OpniReceiver {
	return c
}

func (c *PushoverConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *PushoverConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *SNSConfig) InternalId() string {
	return shared.InternalSNSId
}

func (c *SNSConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	//TODO
	return nil
}

func (c *SNSConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	//TODO
}

func (c *SNSConfig) Configure(*alertingv1.AlertEndpoint) OpniReceiver {
	return c
}

func (c *SNSConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

func (c *SNSConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *TelegramConfig) InternalId() string {
	return shared.InternalTelegramId
}

func (c *TelegramConfig) ExtractMessage() *alertingv1.EndpointImplementation {
	//TODO
	return nil
}

func (c *TelegramConfig) StoreMessage(details *alertingv1.EndpointImplementation) {
	//TODO
}

func (c *TelegramConfig) Configure(*alertingv1.AlertEndpoint) OpniReceiver {
	return c
}

func (c *TelegramConfig) Clone() OpniReceiver {
	return util.DeepCopy(c)
}

func (c *TelegramConfig) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(c)
}
