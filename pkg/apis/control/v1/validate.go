package v1

import (
	"github.com/rancher/opni/pkg/validation"
)

func (a *PluginArchive) Validate() error {
	for _, item := range a.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (a *PluginArchiveEntry) Validate() error {
	if a.Metadata.GetDigest() == "" {
		return validation.Error("digest is required for plugin archive entries")
	}
	if a.Metadata.GetPath() == "" {
		return validation.Error("path is required for plugin archive entries")
	}
	if a.Metadata.GetPackage() == "" {
		return validation.Error("module is required for plugin archive entries")
	}
	if a.Metadata.GetId() == "" {
		return validation.Error("id is required for plugin archive entries")
	}
	return nil
}

func (m *UpdateManifestEntry) Validate() error {
	if m.Digest == "" {
		return validation.Error("digest is required for plugin manifest entries")
	}
	if m.Path == "" {
		return validation.Error("Filename is required for plugin manifest entries")
	}
	if m.Package == "" {
		return validation.Error("Module is required for plugin manifest entries")
	}
	return nil
}

func (m *UpdateManifest) Validate() error {
	for _, item := range m.Items {
		if err := item.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (a *PatchSpec) Validate() error {
	if a.GetFilename() == "" {
		return validation.Error("filename must be set")
	}
	if a.GetModule() == "" {
		return validation.Error("module must be set")
	}
	switch a.GetOp() {
	case PatchOp_Update, PatchOp_Rename:
		if a.GetOldDigest() == "" {
			return validation.Error("OldHash is required for patching")
		}
		if a.GetNewDigest() == "" {
			return validation.Error("NewHash is required for patching")
		}
	case PatchOp_Create:
		if a.GetNewDigest() == "" {
			return validation.Error("NewHash is required for creating")
		}
	case PatchOp_None, PatchOp_Remove:
		if len(a.Data) != 0 {
			return validation.Error("no data should be sent alongside a None/Remove operation")
		}
	}
	return nil
}

func (a *PatchList) Validate() error {
	for _, patch := range a.Items {
		if err := patch.Validate(); err != nil {
			return err
		}
	}
	return nil
}
