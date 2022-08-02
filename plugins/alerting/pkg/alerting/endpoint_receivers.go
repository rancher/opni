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

func (c *ConfigMapData) AppendReceiver(recv *cfg.Receiver) {
	c.Receivers = append(c.Receivers, recv)
}

func (c *ConfigMapData) GetReceivers() []*cfg.Receiver {
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

func (c *ConfigMapData) UpdateReceiver(id string, recv *cfg.Receiver) error {
	if recv == nil {
		return fmt.Errorf("nil receiver passed to UpdateReceiver")
	}
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

func NewSlackReceiver(id string, endpoint *alertingv1alpha.SlackEndpoint) (*cfg.Receiver, error) {
	parsedURL, err := url.Parse(endpoint.ApiUrl)
	if err != nil {
		return nil, err
	}
	// validate the url
	_, err = url.ParseRequestURI(endpoint.ApiUrl)
	if err != nil {
		return nil, err
	}
	channel := strings.TrimSpace(endpoint.Channel)
	if !strings.HasPrefix(channel, "#") {
		//FIXME
		return nil, shared.AlertingErrInvalidSlackChannel
	}

	return &cfg.Receiver{
		Name: id,
		SlackConfigs: []*cfg.SlackConfig{
			{
				APIURL:  &cfg.SecretURL{URL: parsedURL},
				Channel: channel,
			},
		},
	}, nil
}

func WithSlackImplementation(
	cfg *cfg.Receiver,
	impl *alertingv1alpha.SlackImplementation) (*cfg.Receiver, error) {
	if cfg.SlackConfigs == nil || len(cfg.SlackConfigs) == 0 || impl == nil {
		return nil, shared.AlertingErrMismatchedImplementation
	}
	cfg.SlackConfigs[0].Title = impl.Title
	cfg.SlackConfigs[0].Text = impl.Text
	cfg.SlackConfigs[0].Footer = impl.Footer
	cfg.SlackConfigs[0].ImageURL = impl.ImageUrl
	return cfg, nil
}

func NewEmailReceiver(id string, endpoint *alertingv1alpha.EmailEndpoint) (*cfg.Receiver, error) {
	_, err := mail.ParseAddress(endpoint.To)
	if err != nil {
		return nil, validation.Errorf("Invalid Destination email : %w", err)
	}

	if endpoint.From != nil {
		_, err := mail.ParseAddress(*endpoint.From)
		if err != nil {
			return nil, validation.Errorf("Invalid Sender email : %w", err)
		}
	}

	return &cfg.Receiver{
		Name: id,
		EmailConfigs: func() []*cfg.EmailConfig {
			if endpoint.From == nil {
				return []*cfg.EmailConfig{
					{
						To:   endpoint.To,
						From: "alertmanager@localhost",
					},
				}
			}
			return []*cfg.EmailConfig{
				{
					To:   endpoint.To,
					From: *endpoint.From,
				},
			}
		}()}, nil
}

func WithEmailImplementation(cfg *cfg.Receiver, impl *alertingv1alpha.EmailImplementation) (*cfg.Receiver, error) {
	if cfg.EmailConfigs == nil || len(cfg.EmailConfigs) == 0 || impl == nil {
		return nil, shared.AlertingErrMismatchedImplementation
	}
	if impl.TextBody != nil {
		cfg.EmailConfigs[0].Text = *impl.TextBody
	}
	if impl.HtmlBody != nil {
		cfg.EmailConfigs[0].HTML = *impl.HtmlBody
	}

	return cfg, nil
}
