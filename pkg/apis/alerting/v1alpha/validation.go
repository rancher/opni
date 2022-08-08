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
