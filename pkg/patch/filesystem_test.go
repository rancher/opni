package patch_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/patch"
	"github.com/rancher/opni/pkg/test"
)

func init() {
	BuildCacheTestSuite("Filesystem Cache", func() TestCache {
		tmp, err := os.MkdirTemp("", "opni-pkg-cache-test")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			os.RemoveAll(tmp)
		})
		cache, err := patch.NewFilesystemCache(v1beta1.FilesystemCacheSpec{
			Dir: tmp,
		}, patch.BsdiffPatcher{}, test.Log)
		Expect(err).NotTo(HaveOccurred())
		return newTestCache(cache, CacheTestSuiteOptions{
			TestOpenSavedPluginFunc: func(hash string, mode int) (*os.File, error) {
				return os.OpenFile(filepath.Join(tmp, "plugins", hash), mode, 0666)
			},
			TestStatPatchFunc: func(from, to string) (os.FileInfo, error) {
				return os.Stat(filepath.Join(tmp, "patches", cache.PatchKey(from, to)))
			},
			TestRemovePatchFunc: func(from, to string) error {
				return os.Remove(filepath.Join(tmp, "patches", cache.PatchKey(from, to)))
			},
		})
	})
}

var _ = Describe("Filesystem Cache", Ordered, Label("unit"), func() {
	Context("error handling", func() {
		When("creating a new filesystem cache", func() {
			It("should return an error if it cannot create the cache directory", func() {
				tmpDir, err := os.MkdirTemp("", "opni-test-patch")
				Expect(err).NotTo(HaveOccurred())

				Expect(os.Mkdir(filepath.Join(tmpDir, "x"), 0777)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(tmpDir, "x", "plugins"), []byte("foo"), 0644)).To(Succeed())
				Expect(os.WriteFile(filepath.Join(tmpDir, "x", "patches"), []byte("foo"), 0644)).To(Succeed())

				os.Chmod(tmpDir, 0)

				_, err = patch.NewFilesystemCache(v1beta1.FilesystemCacheSpec{
					Dir: filepath.Join(tmpDir, "x"),
				}, patch.BsdiffPatcher{}, test.Log)
				Expect(err).To(HaveOccurred())

				os.Chmod(tmpDir, 0o777)

				_, err = patch.NewFilesystemCache(v1beta1.FilesystemCacheSpec{
					Dir: filepath.Join(tmpDir, "x"),
				}, patch.BsdiffPatcher{}, test.Log)
				Expect(err).To(HaveOccurred())

				os.Remove(filepath.Join(tmpDir, "x", "plugins"))

				_, err = patch.NewFilesystemCache(v1beta1.FilesystemCacheSpec{
					Dir: filepath.Join(tmpDir, "x"),
				}, patch.BsdiffPatcher{}, test.Log)
				Expect(err).To(HaveOccurred())

				os.Remove(filepath.Join(tmpDir, "x", "patches"))

				_, err = patch.NewFilesystemCache(v1beta1.FilesystemCacheSpec{
					Dir: filepath.Join(tmpDir, "x"),
				}, patch.BsdiffPatcher{}, test.Log)
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

})
