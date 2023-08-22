package meta

import (
	"debug/buildinfo"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
)

type PluginMeta struct {
	BinaryPath string
	GoVersion  string
	Module     string

	// Extended metadata not populated from build info.
	ExtendedMetadata *ExtendedPluginMeta
}

type ExtendedPluginMeta struct {
	ModeList ModeList
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

type file interface {
	io.ReaderAt
	Name() string
}

// Reads relevant metadata from an opened file. Does not change the i/o offset
// of the file.
func ReadFile(f file) (PluginMeta, error) {
	info, err := buildinfo.Read(f)
	if err != nil {
		return PluginMeta{}, err
	}
	return PluginMeta{
		BinaryPath: f.Name(),
		GoVersion:  info.GoVersion,
		Module:     info.Path,
	}, nil
}

func ReadMetadata() PluginMeta {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		panic("could not read build info")
	}
	executable, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return PluginMeta{
		BinaryPath: executable,
		GoVersion:  buildInfo.GoVersion,
		Module:     buildInfo.Path,
	}
}
