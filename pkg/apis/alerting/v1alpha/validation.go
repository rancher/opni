package v1alpha

import (
	"github.com/rancher/opni/pkg/alerting/metrics"
	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/validation"
	"golang.org/x/exp/slices"
	"net/mail"
	"net/url"
	"strings"
)

func (s *SilenceRequest) Validate() error {
	if err := s.ConditionId.Validate(); err != nil {
		return err
	}
	if s.Duration == nil || s.Duration.AsDuration() == 0 {
		return validation.Error("Require a non-zero Duration to activate a silence")
	}
	return nil
}

func (s *AlertConditionSystem) Validate() error {
	return nil
}

func (k *AlertConditionKubeState) Validate() error {
	if k.ClusterId == "" {
		return validation.Error("clusterId must be set")
	}
	if k.ObjectType == "" {
		return validation.Error("objectType must be set")
	}
	if k.ObjectName == "" {
		return validation.Error("objectName must be set")
	}
	if k.Namespace == "" {
		return validation.Error("objectNamespace must be set")
	}
	if !slices.Contains(metrics.KubeStates, k.State) {
		return validation.Errorf("state must be one of the following: %v", metrics.KubeStates)
	}
	return nil
}

func (c *AlertConditionComposition) Validate() error {
	return shared.WithUnimplementedErrorf("Composition alerts not implemented yet")
}

func (c *AlertConditionControlFlow) Validate() error {
	return shared.WithUnimplementedErrorf("Control flow alerts not implemented yet")
}

func (d *AlertTypeDetails) Validate() error {
	if d.GetSystem() != nil {
		return d.GetSystem().Validate()
	}
	if d.GetKubeState() != nil {
		return d.GetKubeState().Validate()
	}
	if d.GetComposition() != nil {
		return d.GetComposition().Validate()
	}
	if d.GetControlFlow() != nil {
		return d.GetControlFlow().Validate()
	}
	return validation.Errorf("Unknown alert type provided %v", d)
}

func (a *AlertCondition) Validate() error {
	if a.GetName() == "" {
		return validation.Error("AlertCondition name must be set")
	}
	if err := a.GetAlertType().Validate(); err != nil {
		return err
	}
	if a.NotificationId != nil {
		a.GetDetails().Validate()
	}
	return nil
}

func (e *EndpointImplementation) Validate() error {
	if e.Title == "" {
		return validation.Error("Title must be set")
	}
	if e.Body == "" {
		return validation.Error("Body must be set")
	}
	return nil
}

func (l *ListAlertConditionRequest) Validate() error {
	return nil
}

func (u *UpdateAlertConditionRequest) Validate() error {
	if err := u.GetUpdateAlert().Validate(); err != nil {
		return err
	}
	return nil
}

func (a *AlertDetailChoicesRequest) Validate() error {
	return nil
}

func (a *AlertEndpoint) Validate() error {
	if a.GetName() == "" {
		return validation.Error("AlertEndpoint name must be set")
	}
	if a.GetSlack() != nil {
		return a.GetSlack().Validate()
	}
	if a.GetEmail() != nil {
		return a.GetEmail().Validate()
	}
	return shared.WithUnimplementedErrorf("AlertEndpoint type %v not implemented yet", a)
}

func (s *SlackEndpoint) Validate() error {
	if s.WebhookUrl == "" {
		return validation.Error("webhook must be set")
	}
	// validate the url
	_, err := url.ParseRequestURI(s.WebhookUrl)
	if err != nil {
		return validation.Errorf("webhook must be a valid url : %s", err)
	}

	if s.Channel != "" {
		if !strings.HasPrefix(s.Channel, "#") {
			return validation.Error(shared.AlertingErrInvalidSlackChannel.Error())
		}
	}
	return nil
}

func (e *EmailEndpoint) Validate() error {
	if e.To == "" {
		return validation.Error("email recipient must be set")
	}
	_, err := mail.ParseAddress(e.To)
	if err != nil {
		return validation.Errorf("Invalid recipient email %s", err)
	}
	if e.SmtpFrom != nil && *e.SmtpFrom != "" {
		_, err = mail.ParseAddress(*e.SmtpFrom)
		if err != nil {
			return validation.Errorf("Invalid sender email %s", err)
		}
	}
	if e.SmtpSmartHost != nil {
		arr := strings.Split(*e.SmtpSmartHost, ":")
		if len(arr) != 2 {
			return validation.Errorf("Invalid smtp host %s", *e.SmtpSmartHost)
		}
	}
	return nil
}

func (l *ListAlertEndpointsRequest) Validate() error {
	return nil
}

func (u *UpdateAlertEndpointRequest) Validate() error {
	if err := u.GetUpdateAlert().Validate(); err != nil {
		return err
	}
	return nil
}

func (c *CreateImplementation) Validate() error {
	if err := c.GetImplementation().Validate(); err != nil {
		return err
	}
	return nil
}

func (t *TestAlertEndpointRequest) Validate() error {
	if err := t.GetImpl().Validate(); err != nil {
		return err
	}
	if err := t.GetEndpointInfo().Validate(); err != nil {
		return err
	}
	return nil
}
