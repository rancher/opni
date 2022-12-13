package patch_test

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/patch"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util"
)

func NewDigest4() (string, string, string, string) {
	return uuid.New().String(), uuid.New().String(), uuid.New().String(), uuid.New().String()
}

type testLeftJoinData struct {
	leftConfig  *controlv1.PluginManifest
	rightConfig *controlv1.PluginManifest
	expected    *controlv1.PatchList
}

var _ = Describe("Patch Manifest Operations", func() {
	When("Receiving two sets of manifests", func() {
		hash1, hash2, hash3, hash4 := NewDigest4()

		metrics := &controlv1.PluginManifest{
			Items: []*controlv1.PluginManifestEntry{
				{
					Module:   "metrics",
					Filename: "plugin_metrics",
					Digest:   hash1,
				},
			},
		}
		metricsRenamed := func() *controlv1.PluginManifest {
			t := util.ProtoClone(metrics)
			t.Items[0].Filename = "renamed"
			return t
		}()

		metricsRenamedModule := func() *controlv1.PluginManifest {
			t := util.ProtoClone(metrics)
			t.Items[0].Module = "modeltraining"
			t.Items[0].Digest = hash4 //hash is also necessarily changed when module is changed
			return t
		}()

		metricsUpdate := func() *controlv1.PluginManifest {
			t := util.ProtoClone(metrics)
			t.Items[0].Digest = hash2
			return t
		}()

		_ = &controlv1.PluginManifest{
			Items: []*controlv1.PluginManifestEntry{
				{
					Module:   "metrics",
					Filename: "plugin_metrics",
					Digest:   hash3,
				},
			},
		}

		addlogging := &controlv1.PluginManifest{
			Items: []*controlv1.PluginManifestEntry{
				metrics.Items[0],
				{
					Module:   "logging",
					Filename: "plugin_logging",
					Digest:   hash4,
				},
			},
		}

		removeLogging := &controlv1.PluginManifest{
			Items: []*controlv1.PluginManifestEntry{
				addlogging.Items[0],
			},
		}
		It("should determine when to update plugins", func() {
			// only the hashes differ
			scenario1 := testLeftJoinData{
				leftConfig:  metricsUpdate,
				rightConfig: metrics,
				expected: &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp_Update,
							Module:    metricsUpdate.Items[0].GetModule(),
							OldDigest: metrics.Items[0].GetDigest(),
							NewDigest: metricsUpdate.Items[0].GetDigest(),
							Filename:  metricsUpdate.Items[0].GetFilename(),
						},
					},
				},
			}
			res := patch.LeftJoinOn(scenario1.leftConfig, scenario1.rightConfig)
			By("checking that the output from the two manifests is well formed")
			Expect(res.Validate()).To(Succeed())
			Expect(len(res.Items)).To(Equal(len(scenario1.expected.Items)))
			for i, item := range res.Items {
				By("checking that the generated operation is update")
				Expect(item.Op).To(Equal(controlv1.PatchOp_Update))
				By("checking that the metadata is generated correctly")
				Expect(item.Module).To(Equal(scenario1.expected.Items[i].Module))
				Expect(item.NewDigest).To(Equal(scenario1.expected.Items[i].NewDigest))
				Expect(item.OldDigest).To(Equal(scenario1.expected.Items[i].OldDigest))
				Expect(item.Filename).To(Equal(scenario1.expected.Items[i].Filename))
			}
		})

		It("should determine when to rename plugins", func() {
			scenario2 := testLeftJoinData{
				leftConfig:  metricsRenamed,
				rightConfig: metrics,
				expected: &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp_Rename,
							Module:    metrics.Items[0].GetModule(),
							OldDigest: metrics.Items[0].GetDigest(),
							NewDigest: metricsRenamed.Items[0].GetDigest(),
							Filename:  "plugin_metrics",
							Data:      []byte("renamed"),
						},
					},
				},
			}
			res := patch.LeftJoinOn(scenario2.leftConfig, scenario2.rightConfig)
			By("checking that the output from the two manifests is well formed")
			Expect(res.Validate()).To(Succeed())
			Expect(len(res.Items)).To(Equal(len(scenario2.expected.Items)))
			for i, item := range res.Items {
				By("checking that the generated operation is rename")
				Expect(item.Op).To(Equal(controlv1.PatchOp_Rename))
				By("checking that the metadata is generated correctly")
				Expect(item.Module).To(Equal(scenario2.expected.Items[i].Module))
				Expect(item.NewDigest).To(Equal(scenario2.expected.Items[i].NewDigest))
				Expect(item.OldDigest).To(Equal(scenario2.expected.Items[i].OldDigest))
				Expect(item.Filename).To(Equal(scenario2.expected.Items[i].Filename))
			}
		})

		It("should determine when to add plugins", func() {
			scenario3 := testLeftJoinData{
				leftConfig:  addlogging,
				rightConfig: metricsUpdate,
				expected: &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp_Update,
							Module:    addlogging.Items[0].GetModule(),
							OldDigest: metricsUpdate.Items[0].GetDigest(),
							NewDigest: addlogging.Items[0].GetDigest(),
							Filename:  addlogging.Items[0].GetFilename(),
						},
						{
							Op:        controlv1.PatchOp_Create,
							Module:    addlogging.Items[1].GetModule(),
							OldDigest: "",
							NewDigest: addlogging.Items[1].GetDigest(),
							Filename:  addlogging.Items[1].GetFilename(),
						},
					},
				},
			}
			res := patch.LeftJoinOn(scenario3.leftConfig, scenario3.rightConfig)
			By("checking that the output from the two manifests is well formed")
			Expect(res.Validate()).To(Succeed())
			Expect(len(res.Items)).To(Equal(len(scenario3.expected.Items)))
			for i, item := range res.Items {
				By("checking that the generated operation is create or update")
				Expect(item.Op).To(Equal(scenario3.expected.Items[i].Op))
				By("checking that the metadata is generated correctly")
				Expect(item.Module).To(Equal(scenario3.expected.Items[i].Module))
				Expect(item.NewDigest).To(Equal(scenario3.expected.Items[i].NewDigest))
				Expect(item.OldDigest).To(Equal(scenario3.expected.Items[i].OldDigest))
				Expect(item.Filename).To(Equal(scenario3.expected.Items[i].Filename))
			}
		})

		It("should determine when to remove plugins", func() {
			scenario4 := testLeftJoinData{
				leftConfig:  removeLogging,
				rightConfig: addlogging,
				expected: &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp_Remove,
							Module:    addlogging.Items[1].GetModule(),
							OldDigest: addlogging.Items[1].GetDigest(),
							NewDigest: "",
							Filename:  addlogging.Items[1].GetFilename(),
						},
					},
				},
			}
			res := patch.LeftJoinOn(scenario4.leftConfig, scenario4.rightConfig)
			By("checking that the output from the two manifests is well formed")
			Expect(res.Validate()).To(Succeed())
			Expect(len(res.Items)).To(Equal(len(scenario4.expected.Items)))
			for i, item := range res.Items {
				By("checking that the generated operation is remove")
				Expect(item.Op).To(Equal(controlv1.PatchOp_Remove))
				By("checking that the metadata is generated correctly")
				Expect(item.Module).To(Equal(scenario4.expected.Items[i].Module))
				Expect(item.NewDigest).To(Equal(scenario4.expected.Items[i].NewDigest))
				Expect(item.OldDigest).To(Equal(scenario4.expected.Items[i].OldDigest))
				Expect(item.Filename).To(Equal(scenario4.expected.Items[i].Filename))
			}
		})

		It("should determine when to replace plugins with new contents", func() {
			scenario5 := testLeftJoinData{
				leftConfig:  metricsRenamedModule,
				rightConfig: metrics,
				expected: &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp_Create,
							Module:    metricsRenamedModule.Items[0].GetModule(),
							OldDigest: "",
							NewDigest: metricsRenamedModule.Items[0].GetDigest(),
							Filename:  metricsRenamedModule.Items[0].GetFilename(),
						},
						{
							Op:        controlv1.PatchOp_Remove,
							Module:    metrics.Items[0].GetModule(),
							OldDigest: metrics.Items[0].GetDigest(),
							NewDigest: "",
							Filename:  metrics.Items[0].GetFilename(),
						},
					},
				},
			}
			res := patch.LeftJoinOn(scenario5.leftConfig, scenario5.rightConfig)
			By("checking that the output from the two manifests is well formed")
			Expect(res.Validate()).To(Succeed())
			Expect(len(res.Items)).To(Equal(len(scenario5.expected.Items)))
			for i, item := range res.Items {
				By("checking that the generated operation is create or remove")
				Expect(item.Op).To(Equal(scenario5.expected.Items[i].Op))
				By("checking that the metadata is generated correctly")
				Expect(item.Module).To(Equal(scenario5.expected.Items[i].Module))
				Expect(item.NewDigest).To(Equal(scenario5.expected.Items[i].NewDigest))
				Expect(item.OldDigest).To(Equal(scenario5.expected.Items[i].OldDigest))
				Expect(item.Filename).To(Equal(scenario5.expected.Items[i].Filename))
			}
		})
	})
})

var _ = Describe("Filesystem Discovery", Ordered, func() {
	It("should discover plugins from the filesystem", func() {
		tmpDir, err := os.MkdirTemp("", "opni-test-patch-fs")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() {
			os.RemoveAll(tmpDir)
		})

		Expect(os.Link(*test1v1BinaryPath, filepath.Join(tmpDir, "plugin_test1"))).To(Succeed())
		Expect(os.Link(*test2v1BinaryPath, filepath.Join(tmpDir, "plugin_test2"))).To(Succeed())

		mv1, err := patch.GetFilesystemPlugins(plugins.DiscoveryConfig{
			Dir:    tmpDir,
			Logger: test.Log,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(mv1.Items).To(HaveLen(2))
		Expect(mv1.Items[0].Metadata.Module).To(Equal(test1Module))
		Expect(mv1.Items[1].Metadata.Module).To(Equal(test2Module))
		Expect(mv1.Items[0].Metadata.Digest).To(Equal(v1Manifest.Items[0].Metadata.Digest))
		Expect(mv1.Items[1].Metadata.Digest).To(Equal(v1Manifest.Items[1].Metadata.Digest))
		Expect(mv1.Items[0].Metadata.Filename).To(Equal("plugin_test1"))
		Expect(mv1.Items[1].Metadata.Filename).To(Equal("plugin_test2"))
	})
	When("a plugin has invalid contents", func() {
		It("should log an error and skip it", func() {
			tmpDir, err := os.MkdirTemp("", "opni-test-patch-fs")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				os.RemoveAll(tmpDir)
			})

			Expect(os.Link(*test1v1BinaryPath, filepath.Join(tmpDir, "plugin_test1"))).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "plugin_test2"), []byte("invalid"), 0755)).To(Succeed())

			mv1, err := patch.GetFilesystemPlugins(plugins.DiscoveryConfig{
				Dir:    tmpDir,
				Logger: test.Log,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mv1.Items).To(HaveLen(1))
			Expect(mv1.Items[0].Metadata.Module).To(Equal(test1Module))
		})
	})
	When("a plugin cannot be opened for reading", func() {
		It("should log an error and skip it", func() {
			tmpDir, err := os.MkdirTemp("", "opni-test-patch-fs")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() {
				os.RemoveAll(tmpDir)
			})

			Expect(os.Link(*test1v1BinaryPath, filepath.Join(tmpDir, "plugin_test1"))).To(Succeed())
			test2Data, err := os.ReadFile(*test2v1BinaryPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(tmpDir, "plugin_test2"), test2Data, 0000)).To(Succeed())

			mv1, err := patch.GetFilesystemPlugins(plugins.DiscoveryConfig{
				Dir:    tmpDir,
				Logger: test.Log,
			})

			Expect(err).NotTo(HaveOccurred())
			Expect(mv1.Items).To(HaveLen(1))

			Expect(mv1.Items[0].Metadata.Module).To(Equal(test1Module))
		})
	})
})
