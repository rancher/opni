package patch

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
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
	Patch(archive *controlv1.PluginArchive) error
}

type patchClient struct {
	defaultPluginsDir string
	lg                *zap.SugaredLogger
}

func NewPatchClient(config v1beta1.PluginsSpec, lg *zap.SugaredLogger) (PatchClient, error) {
	if len(config.Dirs) == 0 {
		return nil, fmt.Errorf("no plugin directories configured")
	}
	defaultPluginsDir := config.Dirs[0]
	if len(config.Dirs) > 1 {
		lg.Infof("multiple plugin directories configured, will write plugins to %s", defaultPluginsDir)
	}
	if _, err := os.Stat(defaultPluginsDir); os.IsNotExist(err) {
		if err := os.MkdirAll(defaultPluginsDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create plugin directory %s: %w", defaultPluginsDir, err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat plugin directory %s: %w", defaultPluginsDir, err)
	}
	return &patchClient{
		defaultPluginsDir: defaultPluginsDir,
		lg:                lg,
	}, nil
}

// Patch applies the patch operations contained in the plugin archive to the
// local plugin set defined by the plugin configuration.
// This function returns grpc error codes; codes.Unavailable indicates a
// potentially transient error and that the caller may retry.
func (pc *patchClient) Patch(archive *controlv1.PluginArchive) error {
	group := errgroup.Group{}
	for _, entry := range archive.Items {
		entry := entry
		if entry.AgentPath == "" {
			entry.AgentPath = pc.inferIncompletePath(entry)
		}
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

func (pc *patchClient) inferIncompletePath(entry *controlv1.PluginArchiveEntry) string {
	return filepath.Join(pc.defaultPluginsDir, entry.ShortName)
}

func (pc *patchClient) doRename(entry *controlv1.PluginArchiveEntry) error {
	oldPath := entry.AgentPath
	newPath := path.Join(path.Dir(entry.AgentPath), entry.GetShortName())

	if _, err := os.Stat(newPath); err == nil {
		return unavailableErrf("could not rename plugin %s: destination %s already exists", oldPath, newPath)
	}
	pc.lg.Infof("renaming plugin: %s -> %s", oldPath, newPath)
	err := os.Rename(oldPath, newPath)
	if err != nil {
		return osErrf("could not rename plugin %s: %v", oldPath, err)
	}
	return nil
}

func (pc *patchClient) doCreate(entry *controlv1.PluginArchiveEntry) error {
	pc.lg.With(
		"path", entry.AgentPath,
		"size", len(entry.Data),
	).Infof("writing new plugin")
	// ensure the file does not exist
	if _, err := os.Stat(entry.AgentPath); err == nil {
		return unavailableErrf("could not write plugin %s: file already exists", entry.AgentPath)
	}
	err := os.WriteFile(entry.AgentPath, entry.Data, 0755)
	if err != nil {
		return osErrf("could not write plugin %s: %v", entry.AgentPath, err)
	}
	return nil
}

func (pc *patchClient) doUpdate(entry *controlv1.PluginArchiveEntry) error {
	pc.lg.With(
		"path", entry.AgentPath,
		"size", len(entry.Data),
		"from", entry.GetOldDigest(),
		"to", entry.GetNewDigest(),
	).Infof("updating plugin")
	oldPluginInfo, err := os.Stat(entry.AgentPath)
	if err != nil {
		return osErrf("failed to stat plugin %s: %v", entry.AgentPath, err)
	}
	oldPlugin, err := os.Open(entry.AgentPath)
	if err != nil {
		return osErrf("failed to read plugin %s: %v", entry.AgentPath, err)
	}
	oldDigest, _ := blake2b.New256(nil)
	oldPluginData := bytes.NewBuffer(make([]byte, 0, oldPluginInfo.Size()))
	if _, err := io.Copy(io.MultiWriter(oldDigest, oldPluginData), oldPlugin); err != nil {
		return osErrf("failed to read plugin %s: %v", entry.AgentPath, err)
	}
	if hex.EncodeToString(oldDigest.Sum(nil)) != entry.GetOldDigest() {
		return unavailableErrf("existing plugin %s is invalid, cannot apply patch", entry.Module)
	}

	f, err := renameio.NewPendingFile(entry.AgentPath, renameio.WithPermissions(0755))
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
			return status.Errorf(codes.Internal, "could not write to plugin %s: %v", entry.AgentPath, err)
		}
		return status.Errorf(codes.Unavailable, "could not write to plugin %s: %v", entry.AgentPath, err)
	}
	return nil
}

func (pc *patchClient) doRemove(entry *controlv1.PluginArchiveEntry) error {
	pc.lg.Infof("removing plugin: %s", entry.AgentPath)
	err := os.Remove(entry.AgentPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return osErrf("could not remove plugin %s: %v", entry.AgentPath, err)
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
