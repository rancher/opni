package alerting

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"

	cfg "github.com/prometheus/alertmanager/config"
	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/validation"
	"golang.org/x/exp/slices"
)

func (c *ConfigMapData) AppendReceiver(recv *Receiver) {
	c.Receivers = append(c.Receivers, recv)
}

func (c *ConfigMapData) GetReceivers() []*Receiver {
	return c.Receivers
}

// Assumptions:
// - Id is unique among receivers
// - Receiver Name corresponds with Ids one-to-one
func (c *ConfigMapData) findReceivers(id string) (int, error) {
	foundIdx := -1
	for idx, r := range c.Receivers {
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

func (c *ConfigMapData) UpdateReceiver(id string, recv *Receiver) error {
	idx, err := c.findReceivers(id)
	if err != nil {
		return err
	}
	c.Receivers[idx] = recv
	return nil
}

func (c *ConfigMapData) DeleteReceiver(id string) error {
	idx, err := c.findReceivers(id)
	if err != nil {
		return err
	}
	c.Receivers = slices.Delete(c.Receivers, idx, idx+1)
	return nil
}

func NewSlackReceiver(id string, endpoint *alertingv1alpha.SlackEndpoint) (*Receiver, error) {
	parsedURL, err := url.Parse(endpoint.WebhookUrl)
	if err != nil {
		return nil, err
	}
	// validate the url
	_, err = url.ParseRequestURI(endpoint.WebhookUrl)
	if err != nil {
		return nil, err
	}
	channel := strings.TrimSpace(endpoint.Channel)
	if !strings.HasPrefix(channel, "#") {
		return nil, shared.AlertingErrInvalidSlackChannel
	}

	return &Receiver{
		Name: id,
		SlackConfigs: []*SlackConfig{
			{
				APIURL:  parsedURL.String(),
				Channel: channel,
			},
		},
	}, nil
}

func WithSlackImplementation(
	recv *Receiver,
	impl *alertingv1alpha.EndpointImplementation,
) (*Receiver, error) {
	if recv.SlackConfigs == nil || len(recv.SlackConfigs) == 0 || impl == nil {
		return nil, shared.AlertingErrMismatchedImplementation
	}
	if impl.SendResolved != nil {
		recv.SlackConfigs[0].NotifierConfig = cfg.NotifierConfig{
			VSendResolved: *impl.SendResolved,
		}
	} else {
		recv.SlackConfigs[0].NotifierConfig = cfg.NotifierConfig{
			VSendResolved: false,
		}
	}
	recv.SlackConfigs[0].Title = impl.Title
	recv.SlackConfigs[0].Text = impl.Body

	return recv, nil
}

func NewEmailReceiver(id string, endpoint *alertingv1alpha.EmailEndpoint) (*Receiver, error) {
	resRecv := &Receiver{
		Name:         id,
		EmailConfigs: []*EmailConfig{{}},
	}
	//TODO: move validation to validation.go
	_, err := mail.ParseAddress(endpoint.To)
	if err != nil {
		return nil, validation.Errorf("Invalid Destination email : %w", err)
	}
	if endpoint.SmtpFrom != nil {
		_, err := mail.ParseAddress(*endpoint.SmtpFrom)
		if err != nil {
			return nil, validation.Errorf("Invalid Sender email : %w", err)
		}
		resRecv.EmailConfigs[0].From = *endpoint.SmtpFrom
	}
	resRecv.EmailConfigs[0].To = endpoint.To

	// TODO validate the smtp server url
	if endpoint.SmtpSmartHost != nil {
		// TODO: parse
		arr := strings.Split(*endpoint.SmtpSmartHost, ":")
		if len(arr) != 2 {
			return nil, fmt.Errorf("invalid format for smart host")
		}
		resRecv.EmailConfigs[0].Smarthost = cfg.HostPort{
			Host: arr[0],
			Port: arr[1],
		}
	} else {
		//TODO: fetch opni-deployed smtp server from alertingOptions with deadline context
		resRecv.EmailConfigs[0].Smarthost = cfg.HostPort{
			Host: "localhost",
			Port: "25",
		}
	}
	if endpoint.SmtpAuthUsername != nil {
		resRecv.EmailConfigs[0].AuthUsername = *endpoint.SmtpAuthUsername
	}
	if endpoint.SmtpAuthPassword != nil {
		resRecv.EmailConfigs[0].AuthPassword = *endpoint.SmtpAuthPassword
	}
	if endpoint.SmtpAuthIdentity != nil {
		resRecv.EmailConfigs[0].AuthIdentity = *endpoint.SmtpAuthIdentity
	}
	resRecv.EmailConfigs[0].RequireTLS = endpoint.SmtpRequireTLS
	return resRecv, nil
}

func WithEmailImplementation(recv *Receiver, impl *alertingv1alpha.EndpointImplementation) (*Receiver, error) {
	if recv.EmailConfigs == nil || len(recv.EmailConfigs) == 0 || impl == nil {
		return nil, shared.AlertingErrMismatchedImplementation
	}
	if impl.SendResolved != nil {
		recv.SlackConfigs[0].NotifierConfig = cfg.NotifierConfig{
			VSendResolved: *impl.SendResolved,
		}
	} else {
		recv.SlackConfigs[0].NotifierConfig = cfg.NotifierConfig{
			VSendResolved: false,
		}
	}
	if recv.EmailConfigs[0].Headers == nil {
		recv.EmailConfigs[0].Headers = map[string]string{}
	}
	recv.EmailConfigs[0].Headers["Subject"] = impl.Title
	recv.EmailConfigs[0].HTML = impl.Body
	return recv, nil
}

// NewWebhookReceiver creates a new receiver for the webhook endpoint
func NewWebhookReceiver(id string, endpoint *alertingv1alpha.WebhookEndpoint) (*Receiver, error) {
	parsedURL, err := url.Parse(endpoint.Url)
	if err != nil {
		return nil, err
	}
	// validate the url
	_, err = url.ParseRequestURI(endpoint.Url)
	if err != nil {
		return nil, err
	}

	return &Receiver{
		Name: id,
		WebhookConfigs: []*cfg.WebhookConfig{
			{
				URL: &cfg.URL{URL: parsedURL},
			},
		},
	}, nil
}

// WithWebhookImplementation
//
// As opposed to the slack & email implementations, the information
// sent in this one must be constructed in the annotations of the sender
//
// Since annotations are controlled by the Post Alert API,
// need to make sure we store these annotations somewhere
func (p *PostableAlert) WithWebhookImplementation(impl *alertingv1alpha.EndpointImplementation) error {
	//if impl == nil {
	//	return shared.AlertingErrMismatchedImplementation
	//}
	//p.WithRuntimeInfo("")
	//p.Annotations["summary"] = impl.Title
	//p.Annotations["description"] = impl.Body
	//return nil
	return nil
}
