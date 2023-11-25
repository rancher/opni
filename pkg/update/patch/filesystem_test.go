package patch_test

import (
	"io/fs"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/memfs"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/samber/lo"
	"github.com/spf13/afero"

	configv1 "github.com/rancher/opni/pkg/config/v1"
	"github.com/rancher/opni/pkg/update/patch"
)

func init() {
	BuildCacheTestSuite("Filesystem Cache", func() TestCache {
		fsys := afero.NewMemMapFs()
		cache, err := patch.NewFilesystemCache(fsys, &configv1.FilesystemCacheSpec{
			Dir: lo.ToPtr("/tmp"),
		}, patch.BsdiffPatcher{}, testlog.Log)
		Expect(err).NotTo(HaveOccurred())
		return newTestCache(cache, CacheTestSuiteOptions{
			TestOpenSavedPluginFunc: func(hash string, mode int) (afero.File, error) {
				return fsys.OpenFile(filepath.Join("/tmp", patch.PluginsDir, hash), mode, 0666)
			},
			TestStatPatchFunc: func(from, to string) (fs.FileInfo, error) {
				return fsys.Stat(filepath.Join("/tmp", patch.PatchesDir, cache.PatchKey(from, to)))
			},
			TestRemovePatchFunc: func(from, to string) error {
				return fsys.Remove(filepath.Join("/tmp", patch.PatchesDir, cache.PatchKey(from, to)))
			},
		})
	})
}

var _ = Describe("Filesystem Cache", Label("unit"), func() {
	Context("error handling", func() {
		When("creating a new filesystem cache", func() {
			It("should return an error if it cannot create the cache directory", func() {
				fs := afero.Afero{
					Fs: memfs.NewModeAwareMemFs(),
				}

				tmpDir := "/tmp"

				Expect(fs.MkdirAll(filepath.Join(tmpDir, "x"), 0777)).To(Succeed())
				Expect(fs.WriteFile(filepath.Join(tmpDir, "x", patch.PluginsDir), []byte("foo"), 0644)).To(Succeed())
				Expect(fs.WriteFile(filepath.Join(tmpDir, "x", patch.PatchesDir), []byte("foo"), 0644)).To(Succeed())

				Expect(fs.Chmod(filepath.Join(tmpDir, "x"), 0)).To(Succeed())

				_, err := patch.NewFilesystemCache(fs, &configv1.FilesystemCacheSpec{
					Dir: lo.ToPtr(filepath.Join(tmpDir, "x")),
				}, patch.BsdiffPatcher{}, testlog.Log)
				Expect(err).To(HaveOccurred())

				Expect(fs.Chmod(filepath.Join(tmpDir, "x"), 0o777)).To(Succeed())

				_, err = patch.NewFilesystemCache(fs, &configv1.FilesystemCacheSpec{
					Dir: lo.ToPtr(filepath.Join(tmpDir, "x")),
				}, patch.BsdiffPatcher{}, testlog.Log)
				Expect(err).To(HaveOccurred())

				Expect(fs.Remove(filepath.Join(tmpDir, "x", patch.PluginsDir))).To(Succeed())

				_, err = patch.NewFilesystemCache(fs, &configv1.FilesystemCacheSpec{
					Dir: lo.ToPtr(filepath.Join(tmpDir, "x")),
				}, patch.BsdiffPatcher{}, testlog.Log)
				Expect(err).To(HaveOccurred())

				Expect(fs.Remove(filepath.Join(tmpDir, "x", patch.PatchesDir))).To(Succeed())

				_, err = patch.NewFilesystemCache(fs, &configv1.FilesystemCacheSpec{
					Dir: lo.ToPtr(filepath.Join(tmpDir, "x")),
				}, patch.BsdiffPatcher{}, testlog.Log)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})
