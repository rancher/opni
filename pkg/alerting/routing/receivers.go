package routing

import (
	"fmt"
	"github.com/rancher/opni/pkg/validation"
	"net/url"
	"strings"

	cfg "github.com/prometheus/alertmanager/config"
	alertingv1alpha "github.com/rancher/opni/plugins/alerting/pkg/apis/common"
	"golang.org/x/exp/slices"
)

const SlackEndpointInternalId = "slack"
const EmailEndpointInternalId = "email"

func (r *Receiver) AddEndpoint(
	alertEndpoint *alertingv1alpha.AlertEndpoint,
	details *alertingv1alpha.EndpointImplementation) (int, string, error) {
	if details == nil {
		return -1, "", validation.Errorf("nil endpoint details")
	}
	if s := alertEndpoint.GetSlack(); s != nil {
		slackCfg, err := NewSlackReceiverNode(s)
		if err != nil {
			return -1, "", err
		}
		slackCfg, err = WithSlackImplementation(slackCfg, details)
		if err != nil {
			return -1, "", err
		}
		r.SlackConfigs = append(r.SlackConfigs, slackCfg)
		return len(r.SlackConfigs) - 1, SlackEndpointInternalId, nil
	}
	if e := alertEndpoint.GetEmail(); e != nil {
		emailCfg, err := NewEmailReceiverNode(e)
		if err != nil {
			return -1, "", err
		}
		emailCfg, err = WithEmailImplementation(emailCfg, details)
		if err != nil {
			return -1, "", err
		}
		r.EmailConfigs = append(r.EmailConfigs, emailCfg)
		return len(r.EmailConfigs) - 1, EmailEndpointInternalId, nil
	}
	return -1, "", validation.Errorf("unknown endpoint type : %v", alertEndpoint)
}

func (r *RoutingTree) AppendReceiver(recv *Receiver) {
	r.Receivers = append(r.Receivers, recv)
}

func (r *RoutingTree) GetReceivers() []*Receiver {
	return r.Receivers
}

// NewReceiverBase has to have Name be conditionId
func NewReceiverBase(conditionId string) *Receiver {
	return &Receiver{
		Name: conditionId,
	}
}

// Assumptions:
// - id is unique among receivers
// - Receiver Name corresponds with Ids one-to-one
func (r *RoutingTree) FindReceivers(id string) (int, error) {
	foundIdx := -1
	for idx, r := range r.Receivers {
		if r.Name == id {
			foundIdx = idx
			break
		}
	}
	if foundIdx < 0 {
		return foundIdx, fmt.Errorf("receiver with id %s not found in alertmanager backend", id)
	}
	return foundIdx, nil
}

func (r *RoutingTree) UpdateReceiver(id string, recv *Receiver) error {
	idx, err := r.FindReceivers(id)
	if err != nil {
		return err
	}
	r.Receivers[idx] = recv
	return nil
}

func (r *RoutingTree) DeleteReceiver(conditionId string) error {
	idx, err := r.FindReceivers(conditionId)
	if err != nil {
		return err
	}
	r.Receivers = slices.Delete(r.Receivers, idx, idx+1)
	return nil
}

func NewSlackReceiverNode(endpoint *alertingv1alpha.SlackEndpoint) (*SlackConfig, error) {
	if endpoint.WebhookUrl == "" {
		return nil, validation.Errorf("slack webhook url is empty")
	}
	parsedURL, err := url.Parse(endpoint.WebhookUrl)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(endpoint.Channel, "#") {
		return nil, validation.Errorf("slack channel must start with #")
	}
	return &SlackConfig{
		APIURL:  parsedURL.String(),
		Channel: endpoint.Channel,
	}, nil
}

func WithSlackImplementation(
	slack *SlackConfig,
	impl *alertingv1alpha.EndpointImplementation,
) (*SlackConfig, error) {
	if def := impl.SendResolved; def != nil {
		slack.NotifierConfig = cfg.NotifierConfig{
			VSendResolved: *def,
		}
	} else {
		slack.NotifierConfig = cfg.NotifierConfig{
			VSendResolved: false,
		}
	}
	slack.Title = impl.Title
	slack.Text = impl.Body
	return slack, nil
}

func NewEmailReceiverNode(endpoint *alertingv1alpha.EmailEndpoint) (*EmailConfig, error) {
	email := &EmailConfig{
		To: endpoint.To,
	}
	if endpoint.SmtpFrom != nil {
		email.From = *endpoint.SmtpFrom
	}
	if endpoint.SmtpSmartHost != nil {
		arr := strings.Split(*endpoint.SmtpSmartHost, ":")
		email.Smarthost = cfg.HostPort{
			Host: arr[0],
			Port: arr[1],
		}
	} // otherwise is set to global default in reconciler
	if endpoint.SmtpAuthUsername != nil {
		email.AuthUsername = *endpoint.SmtpAuthUsername
	}
	if endpoint.SmtpAuthPassword != nil {
		email.AuthPassword = *endpoint.SmtpAuthPassword
	}
	if endpoint.SmtpAuthIdentity != nil {
		email.AuthIdentity = *endpoint.SmtpAuthIdentity
	}
	return email, nil
}

func WithEmailImplementation(email *EmailConfig, impl *alertingv1alpha.EndpointImplementation) (*EmailConfig, error) {
	if def := impl.SendResolved; def != nil {
		email.NotifierConfig = cfg.NotifierConfig{
			VSendResolved: *impl.SendResolved,
		}
	} else {
		email.NotifierConfig = cfg.NotifierConfig{
			VSendResolved: false,
		}
	}

	if email.Headers == nil {
		email.Headers = make(map[string]string)
	}
	email.Headers["Subject"] = impl.Title
	email.HTML = impl.Body
	return email, nil
}
