package v1

import (
	"fmt"

	"github.com/rancher/opni/pkg/validation"
)

func (m *ManifestMetadata) Validate() error {
	return nil
}

func (m *ManifestMetadataList) Validate() error {
	return nil
}

func (m *ManifestList) Validate() error {
	return nil
}

func (c *CompressedManifest) Validate() error {
	return nil
}

func (x *PatchSpec) Validate() error {
	if x.GetOldHash() == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "oldHash")
	}
	if x.GetNewHash() == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "newHash")
	}
	if x.GetPluginName() == "" {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "pluginName")
	}
	if len(x.Patch) == 0 {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "patch")
	}
	return nil
}
