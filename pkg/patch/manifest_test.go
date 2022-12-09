package patch_test

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/patch"
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
		// rename a plugin
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

		scenarios := []testLeftJoinData{
			scenario1,
			scenario2,
			scenario3,
			scenario4,
			scenario5,
		}

		It("should calculate what actions to undertake", func() {
			for i, sc := range scenarios {
				ls := patch.LeftJoinOn(sc.leftConfig, sc.rightConfig)
				Expect(ls.Validate()).To(Succeed())
				Expect(len(ls.Items)).To(Equal(len(sc.expected.Items)))
				for j, data := range ls.Items {
					By(fmt.Sprintf("scenario %d, item %d", i+1, j+1))
					Expect(data.Op).To(Equal(sc.expected.Items[j].Op))
					Expect(data.NewDigest).To(Equal(sc.expected.Items[j].NewDigest))
					Expect(data.OldDigest).To(Equal(sc.expected.Items[j].OldDigest))
					Expect(data.Module).To(Equal(sc.expected.Items[j].Module))
					Expect(data.Filename).To(Equal(sc.expected.Items[j].Filename))
				}
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

		mv1, err := patch.GetFilesystemPlugins(v1beta1.PluginsSpec{
			Dir: tmpDir,
		}, test.Log)
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

			mv1, err := patch.GetFilesystemPlugins(v1beta1.PluginsSpec{
				Dir: tmpDir,
			}, test.Log)
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

			mv1, err := patch.GetFilesystemPlugins(v1beta1.PluginsSpec{
				Dir: tmpDir,
			}, test.Log)

			Expect(err).NotTo(HaveOccurred())
			Expect(mv1.Items).To(HaveLen(1))

			Expect(mv1.Items[0].Metadata.Module).To(Equal(test1Module))
		})
	})
})
