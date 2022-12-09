package v1

import (
	"github.com/rancher/opni/pkg/validation"
)

func (m *PluginManifestEntry) Validate() error {
	// TODO finalize format
	return nil
}

func (m *PluginManifest) Validate() error {
	// TODO finalize format
	return nil
}

func (a *PatchSpec) Validate() error {
	if a.GetOldDigest() == "" {
		return validation.Error("OldHash is required for patching")
	}
	if a.GetNewDigest() == "" {
		return validation.Error("NewHash is required for patching")
	}
	if a.GetFilename() == "" {
		return validation.Error("Short name must be set")
	}
	switch a.GetOp() {
	case PatchOp_Rename:
		if a.GetModule() == "" {
			return validation.Error("module name is required for a rename operation")
		}
	case PatchOp_None, PatchOp_Remove:
		if len(a.Data) != 0 {
			return validation.Error("no data should be sent alongside a None/Remove operation")
		}
	}
	return nil
}

func (a *PatchList) Validate() error {
	// TODO finalize format
	return nil
}

// func (x *PatchSpec) Validate() error {
// 	if x.GetOldHash() == "" {
// 		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "oldHash")
// 	}
// 	if x.GetNewHash() == "" {
// 		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "newHash")
// 	}
// 	if x.GetPluginName() == "" {
// 		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "pluginName")
// 	}
// 	if len(x.Patch) == 0 {
// 		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "patch")
// 	}
// 	return nil
// }
