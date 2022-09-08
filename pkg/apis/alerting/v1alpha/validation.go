package v1alpha

import "github.com/rancher/opni/pkg/validation"

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
	return nil
}

func (c *AlertConditionComposition) Validate() error {
	return nil
}

func (c *AlertConditionControlFlow) Validate() error {
	return nil
}

func (d *AlertTypeDetails) Validate() error {
	return nil
}

func (a *AlertCondition) Validate() error {
	return nil
}

func (l *ListAlertConditionRequest) Validate() error {
	return nil
}

func (u *UpdateAlertConditionRequest) Validate() error {
	return nil
}

func (a *AlertDetailChoicesRequest) Validate() error {
	return nil
}

func (a *AlertEndpoint) Validate() error {
	return nil
}

func (l *ListAlertEndpointsRequest) Validate() error {
	return nil
}

func (u *UpdateAlertEndpointRequest) Validate() error {
	return nil
}

func (t *TestAlertEndpointRequest) Validate() error {
	return nil
}

func (c *CreateImplementation) Validate() error {
	return nil
}
