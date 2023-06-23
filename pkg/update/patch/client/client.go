package client

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/update"
	"github.com/rancher/opni/pkg/update/patch"
	"github.com/rancher/opni/pkg/urn"
	"github.com/spf13/afero"
	"go.uber.org/zap"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type patchClient struct {
	PatchClientOptions
	fs        pluginFs
	lg        *zap.SugaredLogger
	pluginDir string
}

type PatchClientOptions struct {
	baseFs afero.Fs
}

type PatchClientOption func(*PatchClientOptions)

func (o *PatchClientOptions) apply(opts ...PatchClientOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithBaseFS(basefs afero.Fs) PatchClientOption {
	return func(o *PatchClientOptions) {
		o.baseFs = basefs
	}
}

func NewPatchClient(pluginDir string, lg *zap.SugaredLogger, opts ...PatchClientOption) (update.SyncHandler, error) {
	options := PatchClientOptions{
		baseFs: afero.NewOsFs(),
	}
	options.apply(opts...)

	if pluginDir == "" {
		return nil, errors.New("plugin directory is not configured")
	}
	if _, err := options.baseFs.Stat(pluginDir); os.IsNotExist(err) {
		if err := options.baseFs.MkdirAll(pluginDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create plugin directory %s: %w", pluginDir, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat plugin directory %s: %w", pluginDir, err)
	}

	tempDirBase, err := findTempDirBase(options.baseFs, pluginDir)
	if err != nil {
		return nil, err
	}

	tempDir, err := afero.TempDir(options.baseFs, tempDirBase, ".opni-plugins-tmp-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary directory: %w", err)
	}

	return &patchClient{
		fs: pluginFs{
			fs: afero.Afero{
				Fs: options.baseFs,
			},
			dir:     pluginDir,
			tempDir: tempDir,
		},
		lg:        lg,
		pluginDir: pluginDir,
	}, nil
}

// findTempDirBase locates a suitable temporary directory in which to store
// temporary data during patching. The directory must be writable, and
// must be on the same filesystem as the plugin directory.
func findTempDirBase(baseFs afero.Fs, pluginDir string) (string, error) {
	af := afero.Afero{
		Fs: baseFs,
	}
	pathsToCheck := []string{
		"",        // default temp dir
		"/tmp",    // /tmp is preferred, if possible
		pluginDir, // use a subdirectory of the plugin dir as a last resort
	}
	pluginDirInfo, err := af.Stat(pluginDir)
	if err != nil {
		panic("bug: plugin directory does not exist")
	}

	deviceInfoAvailable := false
	var pluginDirDevice uint64
	if sys := pluginDirInfo.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			deviceInfoAvailable = true
			pluginDirDevice = stat.Dev
		}
	}

	for _, candidate := range pathsToCheck {
		// fast path: if Dev() returns a valid Stat_t, we can use it to
		// determine if the two paths are on the same device.
		if deviceInfoAvailable {
			if info, err := af.Stat(candidate); err == nil {
				if sys := info.Sys(); sys != nil {
					if stat, ok := sys.(*syscall.Stat_t); ok {
						if stat.Dev == pluginDirDevice {
							return candidate, nil
						}
					}
				}
			}
		}

		// slow path: try to write a file to a new temp directory and rename it
		// to a file in the plugin directory. If the rename succeeds, the
		// two directories are on the same filesystem.
		if err := func() error {
			path, err := af.TempDir(candidate, ".opni-fs-test-")
			if err != nil {
				return err
			}
			defer af.RemoveAll(path)
			testFile, err := af.TempFile(path, ".fs-test-*")
			if err != nil {
				return err
			}
			testFile.Close()
			err = af.Rename(filepath.Join(path, filepath.Base(testFile.Name())), filepath.Join(pluginDir, filepath.Base(testFile.Name())))
			if err != nil {
				return err
			}
			af.Remove(filepath.Join(pluginDir, filepath.Base(testFile.Name())))
			return nil
		}(); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to find a writable temp directory on the same device as %s (tried: %v)", pluginDir, pathsToCheck)
}

type pluginFs struct {
	fs      afero.Afero
	dir     string
	tempDir string
}

// resolve relative paths to the plugin directory
func (f *pluginFs) resolve(path string) string {
	if path == "" {
		return path
	}
	if path == "." {
		return f.dir
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(f.dir, path)
}

func (f *pluginFs) Stat(path string) (fs.FileInfo, error) {
	return f.fs.Stat(f.resolve(path))
}

func (f *pluginFs) Open(path string) (afero.File, error) {
	return f.fs.Open(f.resolve(path))
}

func (f *pluginFs) OpenFile(path string, flag int, perm os.FileMode) (afero.File, error) {
	return f.fs.OpenFile(f.resolve(path), flag, perm)
}

func (f *pluginFs) WriteFile(path string, data []byte, perm os.FileMode) error {
	return f.fs.WriteFile(f.resolve(path), data, perm)
}

func (f *pluginFs) Remove(path string) error {
	return f.fs.Remove(f.resolve(path))
}

func (f *pluginFs) Rename(oldpath, newpath string) error {
	return f.fs.Rename(f.resolve(oldpath), f.resolve(newpath))
}

func (f *pluginFs) ReadDir() ([]fs.FileInfo, error) {
	return f.fs.ReadDir(f.dir)
}

func (f *pluginFs) TempFile(dir, prefix string) (afero.File, error) {
	return f.fs.TempFile(f.resolve(dir), prefix)
}

func (f *pluginFs) Chmod(path string, mode os.FileMode) error {
	return f.fs.Chmod(f.resolve(path), mode)
}

func (f *pluginFs) Chown(path string, uid, gid int) error {
	return f.fs.Chown(f.resolve(path), uid, gid)
}

func (pc *patchClient) Strategy() string {
	return patch.UpdateStrategy
}

func (pc *patchClient) GetCurrentManifest(_ context.Context) (*controlv1.UpdateManifest, error) {
	archive, err := patch.GetFilesystemPlugins(plugins.DiscoveryConfig{
		Dir:    pc.pluginDir,
		Logger: pc.lg,
	})
	if err != nil {
		return nil, err
	}
	manifest := archive.ToManifest()
	// if there are no items in the manifest add a placeholder
	if len(manifest.GetItems()) == 0 {
		packageURN := urn.NewOpniURN(urn.Plugin, patch.UpdateStrategy, "placeholder")
		manifest.Items = append(manifest.Items, &controlv1.UpdateManifestEntry{
			Package: packageURN.String(),
			Path:    "placeholder",
			Digest:  "placeholder",
		})
	}

	return manifest, nil
}

func (pc *patchClient) HandleSyncResults(_ context.Context, sync *controlv1.SyncResults) error {
	return pc.patch(sync.GetRequiredPatches())
}

// Patch applies the patch operations contained in the plugin archive to the
// local plugin set defined by the plugin configuration, and returns an updated
// plugin manifest.
// This function returns grpc error codes; codes.Unavailable indicates a
// potentially transient error and that the caller may retry.
func (pc *patchClient) patch(patches *controlv1.PatchList) error {
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
					"module", entry.GetPackage(),
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
	newPath := string(entry.Data)
	if _, err := pc.fs.Stat(newPath); err == nil {
		return unavailableErrf("could not rename plugin %s: destination %s already exists", entry.Package, entry.Path)
	}
	pc.lg.Infof("renaming plugin: %s -> %s", entry.Path, newPath)
	err := pc.fs.Rename(entry.Path, newPath)
	if err != nil {
		return osErrf("could not rename plugin %s: %v", entry.Path, err)
	}
	return nil
}

func (pc *patchClient) doCreate(entry *controlv1.PatchSpec) error {
	pc.lg.With(
		"path", entry.Path,
		"size", len(entry.Data),
	).Infof("writing new plugin")
	err := pc.fs.WriteFile(entry.Path, entry.Data, 0755)
	if err != nil {
		return osErrf("could not write plugin %s: %v", entry.Path, err)
	}
	return nil
}

func (pc *patchClient) doUpdate(entry *controlv1.PatchSpec) error {
	pc.lg.With(
		"filename", entry.Path,
		"size", len(entry.Data),
		"from", entry.GetOldDigest(),
		"to", entry.GetNewDigest(),
	).Infof("updating plugin")
	oldPluginInfo, err := pc.fs.Stat(entry.Path)
	if err != nil {
		return osErrf("failed to stat plugin %s: %v", entry.Path, err)
	}
	oldPlugin, err := pc.fs.Open(entry.Path)
	if err != nil {
		return osErrf("failed to read plugin %s: %v", entry.Path, err)
	}
	oldDigest, _ := blake2b.New256(nil)
	oldPluginData := bytes.NewBuffer(make([]byte, 0, oldPluginInfo.Size()))
	if _, err := io.Copy(io.MultiWriter(oldDigest, oldPluginData), oldPlugin); err != nil {
		oldPlugin.Close()
		return osErrf("failed to read plugin %s: %v", entry.Path, err)
	}
	oldPlugin.Close()
	if hex.EncodeToString(oldDigest.Sum(nil)) != entry.GetOldDigest() {
		return unavailableErrf("existing plugin %s is invalid, cannot apply patch", entry.Package)
	}

	patchReader := bytes.NewReader(entry.Data)
	patcher, ok := patch.NewPatcherFromFormat(patchReader)
	if !ok {
		// read up to 16 bytes of the file for diagnostic purposes
		header := make([]byte, 16)
		copy(header, entry.Data)
		pc.lg.With(
			"patchSize", len(entry.Data),
			"header", strings.TrimSpace(hex.Dump(header)),
		).Error("malformed or incompatible patch was received from the server")
		return internalErrf("unknown patch format for plugin %s", entry.Package)
	}

	tmp, err := pc.fs.TempFile(pc.fs.tempDir, ".opni-tmp-plugin-")
	if err != nil {
		return internalErrf("could not create temporary file: %v", err)
	}

	origTempFile := tmp.Name()
	defer func() {
		if _, err := pc.fs.Stat(origTempFile); err == nil {
			pc.fs.Remove(origTempFile)
		}
	}()

	newDigest, _ := blake2b.New256(nil)
	if err := patcher.ApplyPatch(oldPluginData, patchReader, io.MultiWriter(tmp, newDigest)); err != nil {
		tmp.Close()
		return osErrf("failed applying patch for plugin %s: %v", entry.Package, err)
	}
	tmp.Close()
	if hex.EncodeToString(newDigest.Sum(nil)) != entry.GetNewDigest() {
		return status.Errorf(codes.Unavailable, "patch failed for plugin %s (checksum mismatch)", entry.Package)
	}

	// try to chmod the temp file to match the old plugin if possible
	// if not, chmod after the rename

	existingPerm := oldPluginInfo.Mode() & fs.ModePerm
	var tmpChmodOk bool
	if err := pc.fs.Chmod(tmp.Name(), existingPerm); err == nil {
		tmpChmodOk = true
	}

	// replace the old plugin atomically
	if err := pc.fs.Rename(tmp.Name(), entry.Path); err != nil {
		return osErrf("could not write to plugin %s: %v", entry.Path, err)
	}

	if !tmpChmodOk {
		// if we couldn't chmod the temp file for some reason, chmod the new file
		if err := pc.fs.Chmod(entry.Path, existingPerm); err != nil {
			return osErrf("could not update permissions for plugin %s: %v", entry.Path, err)
		}
	}
	return nil
}

func (pc *patchClient) doRemove(entry *controlv1.PatchSpec) error {
	pc.lg.Infof("removing plugin: %s", entry.Path)
	err := pc.fs.Remove(entry.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return osErrf("could not remove plugin %s: %v", entry.Path, err)
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
	if os.IsPermission(err) {
		return internalErrf(format, args...)
	}
	return unavailableErrf(format, args...)
}

func init() {
	update.RegisterPluginSyncHandlerBuilder(patch.UpdateStrategy, func(args ...any) (update.SyncHandler, error) {
		conf, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", args[0])
		}
		lg, ok := args[1].(*zap.SugaredLogger)
		if !ok {
			return nil, fmt.Errorf("expected *zap.Logger, got %T", args[1])
		}

		var opts []PatchClientOption
		for _, arg := range args[2:] {
			switch v := arg.(type) {
			case afero.Fs:
				opts = append(opts, WithBaseFS(v))
			default:
				return nil, fmt.Errorf("unexpected argument type %T", arg)
			}
		}

		return NewPatchClient(conf, lg, opts...)
	})
}
