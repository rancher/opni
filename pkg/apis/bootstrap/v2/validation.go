package v2

import "github.com/rancher/opni/pkg/validation"

func (h *BootstrapAuthRequest) Validate() error {
	if h.ClientId == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "ClientID")
	}
	if err := validation.ValidateID(h.ClientId); err != nil {
		return validation.ErrInvalidID
	}
	if len(h.ClientPubKey) == 0 {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "ClientPubKey")
	}
	if h.FriendlyName != nil {
		if err := validation.ValidateLabelValue(*h.FriendlyName); err != nil {
			return validation.ErrInvalidName
		}
	}
	return nil
}
