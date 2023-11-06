package admin

import (
	"github.com/rancher/opni/pkg/validation"
)

func (a *AISettings) Validate() error {
	return nil
}

func (s *S3Settings) Validate() error {
	if s.Endpoint == "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "Endpoint")
	}

	if s.AccessKey != "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "AccessKey")
	}

	if s.SecretKey != "" {
		return validation.Errorf("%w: %s", validation.ErrMissingRequiredField, "SecretKey")
	}

	if s.NulogBucket != nil && *s.NulogBucket == "" {
		return validation.Error("Nulog bucket name cannot be empty")
	}

	if s.DrainBucket != nil && *s.DrainBucket == "" {
		return validation.Error("Drain bucket name cannot be empty")
	}
	return nil
}
