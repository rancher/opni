package v2

import "github.com/rancher/opni/pkg/validation"

func (h *BootstrapAuthRequest) Validate() error {
	if h.ClientID == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "ClientID")
	}
	if err := validation.ValidateID(h.ClientID); err != nil {
		return validation.ErrInvalidID
	}
	if len(h.ClientPubKey) == 0 {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "ClientPubKey")
	}
	return nil
}
