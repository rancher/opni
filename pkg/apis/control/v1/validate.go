package v1

import (
	"fmt"
	"log/slog"
	"regexp"

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
		return validation.Error("Path is required for plugin manifest entries")
	}
	if m.Package == "" {
		return validation.Error("Package is required for plugin manifest entries")
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
	if a.GetPath() == "" {
		return validation.Error("filename must be set")
	}
	if a.GetPackage() == "" {
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

func (r *LogStreamRequest) Validate() error {
	if r.GetSince() == nil {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "since")
	}
	if r.GetUntil() == nil {
		return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "until")
	}
	since := r.GetSince().AsTime()
	until := r.GetUntil().AsTime()
	if since.After(until) {
		return fmt.Errorf("%w: %s", validation.ErrInvalidValue, "start time must be before end time")
	}

	if r.GetFilters() == nil {
		return nil
	}

	switch slog.Level(r.GetFilters().GetLevel()) {
	case slog.LevelDebug:
	case slog.LevelInfo:
	case slog.LevelWarn:
	case slog.LevelError:
	default:
		return fmt.Errorf("%w: %s", validation.ErrInvalidValue, "log level")
	}

	for _, name := range r.GetFilters().GetNamePattern() {
		if name == "" {
			return fmt.Errorf("%w: %s", validation.ErrMissingRequiredField, "filter pattern")
		}

		if _, err := regexp.Compile(name); err != nil {
			return err
		}
	}
	return nil
}
