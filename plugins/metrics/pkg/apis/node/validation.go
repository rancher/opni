package node

import "github.com/rancher/opni/pkg/validation"

func (s *SyncRequest) Validate() error {
	if s.CurrentConfig == nil {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "CurrentConfig")
	}
	return nil
}
