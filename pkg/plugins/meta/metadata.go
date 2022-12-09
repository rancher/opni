package meta

import (
	"debug/buildinfo"
	"os"
	"path/filepath"
)

type PluginMeta struct {
	BinaryPath string
	GoVersion  string
	Module     string
}

func (pm PluginMeta) Filename() string {
	return filepath.Base(pm.BinaryPath)
}

// Reads relevant metadata from the binary at the given path.
func ReadPath(path string) (PluginMeta, error) {
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

// Reads relevant metadata from an opened file. Does not change the i/o offset
// of the file.
func ReadFile(file *os.File) (PluginMeta, error) {
	info, err := buildinfo.Read(file)
	if err != nil {
		return PluginMeta{}, err
	}
	return PluginMeta{
		BinaryPath: file.Name(),
		GoVersion:  info.GoVersion,
		Module:     info.Path,
	}, nil
}
