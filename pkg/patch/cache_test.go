package patch_test

import (
	"io"
	"os"
	"runtime"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/patch"
)

type TestCache interface {
	patch.Cache
	openSavedPlugin(hash string, mode int) (*os.File, error)
	statPatch(from, to string) (os.FileInfo, error)
	removePatch(from, to string) error
}

type CacheTestSuiteOptions struct {
	TestOpenSavedPluginFunc func(hash string, mode int) (*os.File, error)
	TestStatPatchFunc       func(from, to string) (os.FileInfo, error)
	TestRemovePatchFunc     func(from, to string) error
}

func (o *CacheTestSuiteOptions) openSavedPlugin(hash string, mode int) (*os.File, error) {
	return o.TestOpenSavedPluginFunc(hash, mode)
}

func (o *CacheTestSuiteOptions) statPatch(from, to string) (os.FileInfo, error) {
	return o.TestStatPatchFunc(from, to)
}

func (o *CacheTestSuiteOptions) removePatch(from, to string) error {
	return o.TestRemovePatchFunc(from, to)
}

type testCache struct {
	CacheTestSuiteOptions
	patch.Cache
}

func newTestCache(c patch.Cache, opts CacheTestSuiteOptions) TestCache {
	return &testCache{
		Cache:                 c,
		CacheTestSuiteOptions: opts,
	}
}

func BuildCacheTestSuite(name string, patchConstructor func() TestCache) bool {
	return Describe(name, Ordered, Label("unit"), func() {
		var cache TestCache
		BeforeEach(func() {
			cache = patchConstructor()
		})
		When("archiving a plugin manifest", func() {
			It("should save all plugins", func() {
				By("archiving the first manifest")
				err := cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("listing the plugins")
				plugins, err := cache.ListDigests()
				Expect(err).NotTo(HaveOccurred())
				Expect(plugins).To(ConsistOf(v1Manifest.Items[0].Metadata.Digest, v1Manifest.Items[1].Metadata.Digest))
			})
		})
		When("archiving the same manifest twice", func() {
			It("should do nothing", func() {
				By("archiving the first manifest")
				err := cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("archiving the first manifest again")
				err = cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("listing the plugins")
				plugins, err := cache.ListDigests()
				Expect(err).NotTo(HaveOccurred())
				Expect(plugins).To(ConsistOf(v1Manifest.Items[0].Metadata.Digest, v1Manifest.Items[1].Metadata.Digest))
			})
			When("an archived plugin is modified on disk", func() {
				It("should replace plugins that do not match the hash", func() {
					By("archiving the first manifest")
					err := cache.Archive(v1Manifest)
					Expect(err).NotTo(HaveOccurred())

					By("modifying one of the saved plugins")
					f, err := cache.openSavedPlugin(v1Manifest.Items[0].Metadata.Digest, os.O_WRONLY|os.O_APPEND)
					Expect(err).NotTo(HaveOccurred())
					f.Write([]byte("hello world"))
					f.Close()

					By("archiving the first manifest again")
					err = cache.Archive(v1Manifest)
					Expect(err).NotTo(HaveOccurred())

					By("listing the plugins")
					plugins, err := cache.ListDigests()
					Expect(err).NotTo(HaveOccurred())
					Expect(plugins).To(ConsistOf(v1Manifest.Items[0].Metadata.Digest, v1Manifest.Items[1].Metadata.Digest))

					By("checking the modified plugin")
					f, err = cache.openSavedPlugin(v1Manifest.Items[0].Metadata.Digest, os.O_RDONLY)
					Expect(err).NotTo(HaveOccurred())
					defer f.Close()
					f.Seek(-11, io.SeekEnd)
					buf := make([]byte, 11)
					_, err = f.Read(buf)
					Expect(err).NotTo(HaveOccurred())
					Expect(buf).NotTo(Equal([]byte("hello world")))
				})
			})
		})
		When("archiving a second manifest", func() {
			It("should save all plugins", func() {
				By("archiving the first manifest")
				err := cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("archiving the second manifest")
				err = cache.Archive(v2Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("listing the plugins")
				plugins, err := cache.ListDigests()
				Expect(err).NotTo(HaveOccurred())
				Expect(plugins).To(ConsistOf(v1Manifest.Items[0].Metadata.Digest, v1Manifest.Items[1].Metadata.Digest, v2Manifest.Items[0].Metadata.Digest, v2Manifest.Items[1].Metadata.Digest))
			})
			It("should allow requesting patches", func() {
				By("archiving the first manifest")
				err := cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("archiving the second manifest")
				err = cache.Archive(v2Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("requesting patches")
				patch, err := cache.RequestPatch(v1Manifest.Items[0].Metadata.Digest, v2Manifest.Items[0].Metadata.Digest)
				Expect(err).NotTo(HaveOccurred())
				Expect(patch).To(Equal(test1v1tov2Patch.Bytes()))

				patch, err = cache.RequestPatch(v1Manifest.Items[1].Metadata.Digest, v2Manifest.Items[1].Metadata.Digest)
				Expect(err).NotTo(HaveOccurred())
				Expect(patch).To(Equal(test2v1tov2Patch.Bytes()))
			})
		})
		When("requesting an already cached patch", func() {
			It("should return the existing patch instead of generating a new one", func() {
				By("archiving the first manifest")
				err := cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("archiving the second manifest")
				err = cache.Archive(v2Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("requesting a patch")
				before := cache.MetricsSnapshot()
				from, to := v1Manifest.Items[0].Metadata.Digest, v2Manifest.Items[0].Metadata.Digest
				patch, err := cache.RequestPatch(from, to)
				Expect(err).NotTo(HaveOccurred())
				Expect(patch).To(Equal(test1v1tov2Patch.Bytes()))
				after := cache.MetricsSnapshot()

				By("checking that a cache miss occurred")
				Expect(after.CacheMisses).To(Equal(before.CacheMisses + 1))

				By("requesting the same patch again")
				before = cache.MetricsSnapshot()
				patch, err = cache.RequestPatch(from, to)
				Expect(err).NotTo(HaveOccurred())
				Expect(patch).To(Equal(test1v1tov2Patch.Bytes()))
				after = cache.MetricsSnapshot()

				By("checking that the cache was not hit")
				Expect(after.CacheMisses).To(Equal(before.CacheMisses))
				Expect(after.CacheHits).To(Equal(before.CacheHits + 1))
			})
		})
		When("multiple clients request the same patch at the same time", func() {
			It("should only generate the patch once", func() {
				By("archiving the first manifest")
				err := cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("archiving the second manifest")
				err = cache.Archive(v2Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("requesting patches concurrently")
				start := make(chan struct{})
				beforeMetrics := cache.MetricsSnapshot()
				var wg sync.WaitGroup
				for i := 0; i < 100; i++ {
					wg.Add(1)
					go func() {
						defer GinkgoRecover()
						defer wg.Done()
						patch, err := cache.RequestPatch(v1Manifest.Items[0].Metadata.Digest, v2Manifest.Items[0].Metadata.Digest)
						Expect(err).NotTo(HaveOccurred())
						Expect(patch).To(Equal(test1v1tov2Patch.Bytes()))
					}()
				}
				runtime.Gosched()
				close(start)
				wg.Wait()

				By("checking that only one patch was generated")
				afterMetrics := cache.MetricsSnapshot()
				Expect(afterMetrics.CacheMisses).To(Equal(beforeMetrics.CacheMisses + 1))
				Expect(afterMetrics.CacheHits).To(Equal(beforeMetrics.CacheHits + 99))
				Expect(afterMetrics.TotalSizeBytes).To(Equal(beforeMetrics.TotalSizeBytes + int64(test1v1tov2Patch.Len())))
			})
		})
		When("cleaning a plugin from the cache", func() {
			It("should remove the plugin and any associated patches", func() {
				By("archiving the first manifest")
				err := cache.Archive(v1Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("archiving the second manifest")
				err = cache.Archive(v2Manifest)
				Expect(err).NotTo(HaveOccurred())

				By("requesting a patch")
				patch, err := cache.RequestPatch(v1Manifest.Items[0].Metadata.Digest, v2Manifest.Items[0].Metadata.Digest)
				Expect(err).NotTo(HaveOccurred())
				Expect(patch).To(Equal(test1v1tov2Patch.Bytes()))

				from, to := v1Manifest.Items[0].Metadata.Digest, v2Manifest.Items[0].Metadata.Digest
				// make sure this file exists
				info, err := cache.statPatch(from, to)
				Expect(err).NotTo(HaveOccurred())
				Expect(info).NotTo(BeNil())

				By("cleaning the plugin")
				cache.Clean(v1Manifest.Items[0].Metadata.Digest)

				By("checking the cache")
				// make sure the patch file is gone
				info, err = cache.statPatch(from, to)
				Expect(err).To(HaveOccurred())
				Expect(info).To(BeNil())

				// make sure the plugin file is gone
				_, err = cache.openSavedPlugin(v1Manifest.Items[0].Metadata.Digest, os.O_RDONLY)
				Expect(err).To(HaveOccurred())
			})
		})
	})
}
