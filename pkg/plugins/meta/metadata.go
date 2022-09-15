package meta

import (
	"debug/buildinfo"
	"path/filepath"
)

type PluginMeta struct {
	BinaryPath string
	GoVersion  string
	Module     string
}

func (pm PluginMeta) ShortName() string {
	return filepath.Base(pm.BinaryPath)
}

// Reads relevant metadata from the binary at the given path.
func ReadMetadata(path string) (PluginMeta, error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return PluginMeta{}, err
	}
	return PluginMeta{
		BinaryPath: path,
		GoVersion:  info.GoVersion,
		Module:     info.Path,
	}, nil
}
