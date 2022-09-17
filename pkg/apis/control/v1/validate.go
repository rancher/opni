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
	//var errors []error
	//for pluginPath, c := range m.GetManifests() {
	//	inExpectedFormat := shared.PluginPathRegex().MatchString(pluginPath)
	//	if !inExpectedFormat {
	//		errors = append(
	//			errors,
	//			validation.Errorf(
	//				"expected plugin path to match %s, got %s",
	//				shared.PluginPathRegex().String(),
	//				pluginPath,
	//			))
	//		continue
	//	}
	//	if err := c.Validate(); err != nil {
	//		errors = append(errors, err)
	//	}
	//}
	//if len(errors) > 0 {
	//	return validation.Errorf("encountered the following errors %s", strings.Join(
	//		func() []string {
	//			res := make([]string, len(errors))
	//			for i, e := range errors {
	//				res[i] = e.Error()
	//			}
	//			return res
	//		}(), ","))
	//}
	return nil
}

func (c *CompressedManifest) Validate() error {
	//validFormat := false
	//if b := c.GetBinary(); b != nil {
	//	validFormat = true
	//}
	//if p := c.GetPatch(); p != nil {
	//	validFormat = true
	//}
	//if r := c.GetRemove(); r != nil {
	//	if c.AttachedMetadata != nil {
	//		return validation.Errorf("remove manifest cannot have attached metadata")
	//	}
	//	if len(r.Data) != 0 {
	//		return validation.Error("remove data must be non-empty")
	//	}
	//	validFormat = true
	//}
	//if !validFormat {
	//	return validation.Error("expected compressed manifest to have a binary or patch field")
	//}
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
