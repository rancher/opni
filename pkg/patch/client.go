package patch

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/renameio/v2"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"go.uber.org/zap"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type PatchClient interface {
	Patch(patches *controlv1.PatchList) error
}

type patchClient struct {
	fs pluginFs
	lg *zap.SugaredLogger
}

func NewPatchClient(config v1beta1.PluginsSpec, lg *zap.SugaredLogger) (PatchClient, error) {
	if config.Dir == "" {
		return nil, errors.New("plugin directory is not configured")
	}
	if _, err := os.Stat(config.Dir); os.IsNotExist(err) {
		if err := os.MkdirAll(config.Dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create plugin directory %s: %w", config.Dir, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat plugin directory %s: %w", config.Dir, err)
	}
	return &patchClient{
		fs: pluginFs{
			dir: config.Dir,
		},
		lg: lg,
	}, nil
}

type pluginFs struct {
	dir string
}

func (f *pluginFs) Stat(path string) (os.FileInfo, error) {
	return os.Stat(filepath.Join(f.dir, path))
}

func (f *pluginFs) Open(path string) (*os.File, error) {
	return os.Open(filepath.Join(f.dir, path))
}

func (f *pluginFs) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(filepath.Join(f.dir, path), flag, perm)
}

func (f *pluginFs) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(filepath.Join(f.dir, path), data, perm)
}

func (f *pluginFs) Remove(path string) error {
	return os.Remove(filepath.Join(f.dir, path))
}

func (f *pluginFs) Rename(oldpath, newpath string) error {
	return os.Rename(filepath.Join(f.dir, oldpath), filepath.Join(f.dir, newpath))
}

func (f *pluginFs) ReadDir() ([]fs.DirEntry, error) {
	return os.ReadDir(f.dir)
}
func (f *pluginFs) NewPendingFile(path string, opts ...renameio.Option) (*renameio.PendingFile, error) {
	return renameio.NewPendingFile(filepath.Join(f.dir, path), opts...)
}

// Patch applies the patch operations contained in the plugin archive to the
// local plugin set defined by the plugin configuration, and returns an updated
// plugin manifest.
// This function returns grpc error codes; codes.Unavailable indicates a
// potentially transient error and that the caller may retry.
func (pc *patchClient) Patch(patches *controlv1.PatchList) error {
	group := errgroup.Group{}
	for _, entry := range patches.Items {
		entry := entry
		group.Go(func() error {
			switch entry.GetOp() {
			case controlv1.PatchOp_Create:
				return pc.doCreate(entry)
			case controlv1.PatchOp_Update:
				return pc.doUpdate(entry)
			case controlv1.PatchOp_Remove:
				return pc.doRemove(entry)
			case controlv1.PatchOp_Rename:
				return pc.doRename(entry)
			case controlv1.PatchOp_None:
				// no-op
			default:
				pc.lg.With(
					"op", entry.GetOp(),
					"module", entry.GetModule(),
				).Warn("server requested an unknown patch operation")
				return status.Errorf(codes.Internal, "unknown patch operation %s", entry.GetOp())
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return err
	}
	return nil
}

func (pc *patchClient) doRename(entry *controlv1.PatchSpec) error {
	newFilename := string(entry.Data)
	if _, err := pc.fs.Stat(newFilename); err == nil {
		return unavailableErrf("could not rename plugin %s: destination %s already exists", entry.Module, entry.Filename)
	}
	pc.lg.Infof("renaming plugin: %s -> %s", entry.Filename, newFilename)
	err := pc.fs.Rename(entry.Filename, newFilename)
	if err != nil {
		return osErrf("could not rename plugin %s: %v", entry.Filename, err)
	}
	return nil
}

func (pc *patchClient) doCreate(entry *controlv1.PatchSpec) error {
	pc.lg.With(
		"path", entry.Filename,
		"size", len(entry.Data),
	).Infof("writing new plugin")
	err := pc.fs.WriteFile(entry.Filename, entry.Data, 0755)
	if err != nil {
		return osErrf("could not write plugin %s: %v", entry.Filename, err)
	}
	return nil
}

func (pc *patchClient) doUpdate(entry *controlv1.PatchSpec) error {
	pc.lg.With(
		"filename", entry.Filename,
		"size", len(entry.Data),
		"from", entry.GetOldDigest(),
		"to", entry.GetNewDigest(),
	).Infof("updating plugin")
	oldPluginInfo, err := pc.fs.Stat(entry.Filename)
	if err != nil {
		return osErrf("failed to stat plugin %s: %v", entry.Filename, err)
	}
	oldPlugin, err := pc.fs.Open(entry.Filename)
	if err != nil {
		return osErrf("failed to read plugin %s: %v", entry.Filename, err)
	}
	oldDigest, _ := blake2b.New256(nil)
	oldPluginData := bytes.NewBuffer(make([]byte, 0, oldPluginInfo.Size()))
	if _, err := io.Copy(io.MultiWriter(oldDigest, oldPluginData), oldPlugin); err != nil {
		return osErrf("failed to read plugin %s: %v", entry.Filename, err)
	}
	if hex.EncodeToString(oldDigest.Sum(nil)) != entry.GetOldDigest() {
		return unavailableErrf("existing plugin %s is invalid, cannot apply patch", entry.Module)
	}

	f, err := pc.fs.NewPendingFile(entry.Filename, renameio.WithPermissions(0755))
	if err != nil {
		return internalErrf("could not create temporary file: %v", err)
	}
	defer f.Cleanup()

	patchReader := bytes.NewReader(entry.Data)
	patcher, ok := NewPatcherFromFormat(patchReader)
	if !ok {
		// read up to 16 bytes of the file for diagnostic purposes
		header := make([]byte, 16)
		copy(header, entry.Data)
		pc.lg.With(
			"patchSize", len(entry.Data),
			"header", strings.TrimSpace(hex.Dump(header)),
		).Error("malformed or incompatible patch was received from the server")
		return internalErrf("unknown patch format for plugin %s", entry.Module)
	}

	newDigest, _ := blake2b.New256(nil)
	if err := patcher.ApplyPatch(oldPluginData, patchReader, io.MultiWriter(f, newDigest)); err != nil {
		return osErrf("failed applying patch for plugin %s: %v", entry.Module, err)
	}
	if hex.EncodeToString(newDigest.Sum(nil)) != entry.GetNewDigest() {
		return status.Errorf(codes.Unavailable, "patch failed for plugin %s (checksum mismatch)", entry.Module)
	}
	if err := f.CloseAtomicallyReplace(); err != nil {
		if os.IsPermission(err) {
			return status.Errorf(codes.Internal, "could not write to plugin %s: %v", entry.Filename, err)
		}
		return status.Errorf(codes.Unavailable, "could not write to plugin %s: %v", entry.Filename, err)
	}
	return nil
}

func (pc *patchClient) doRemove(entry *controlv1.PatchSpec) error {
	pc.lg.Infof("removing plugin: %s", entry.Filename)
	err := pc.fs.Remove(entry.Filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return osErrf("could not remove plugin %s: %v", entry.Filename, err)
	}
	return nil
}

func internalErrf(format string, args ...interface{}) error {
	return status.Errorf(codes.Internal, format, args...)
}

func unavailableErrf(format string, args ...interface{}) error {
	return status.Errorf(codes.Unavailable, format, args...)
}

func osErrf(format string, args ...interface{}) error {
	err, ok := args[len(args)-1].(error)
	if !ok || err == nil {
		panic("bug: last argument must be a non-nil error")
	}
	if errors.Is(err, os.ErrPermission) {
		return internalErrf(format, args...)
	}
	return unavailableErrf(format, args...)
}
