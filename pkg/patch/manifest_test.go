package patch_test

import (
	"os"
	"path/filepath"
	"runtime"

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
	expected    *controlv1.PluginArchive
}

var _ = Describe("Patch Manifest Operations", func() {
	When("Receiving two sets of manifests", func() {
		hash1, hash2, hash3, hash4 := NewDigest4()

		metrics := &controlv1.PluginManifest{
			Items: []*controlv1.PluginManifestEntry{
				{
					BinaryPath: "/home/test/plugin_metrics",
					GoVersion:  "1.19.3",
					Module:     "metrics",
					ShortName:  "metrics",
					Digest:     hash1,
				},
			},
		}
		metricsRenamed := func() *controlv1.PluginManifest {
			t := util.ProtoClone(metrics)
			t.Items[0].ShortName = "renamed"
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
					BinaryPath: "/home/test/plugin_metrics",
					GoVersion:  "1.19.3",
					Module:     "metrics",
					ShortName:  "metrics",
					Digest:     hash3,
				},
			},
		}

		addlogging := &controlv1.PluginManifest{
			Items: []*controlv1.PluginManifestEntry{
				metrics.Items[0],
				{
					BinaryPath: "/home/test/plugin_logging",
					GoVersion:  "1.19.3",
					Module:     "logging",
					ShortName:  "logging",
					Digest:     hash4,
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
			expected: &controlv1.PluginArchive{
				Items: []*controlv1.PluginArchiveEntry{
					{
						Op:          controlv1.PatchOp_Update,
						GatewayPath: metricsUpdate.Items[0].GetBinaryPath(),
						AgentPath:   metrics.Items[0].GetBinaryPath(),
						Module:      metricsUpdate.Items[0].GetModule(),
						OldDigest:   metrics.Items[0].GetDigest(),
						NewDigest:   metricsUpdate.Items[0].GetDigest(),
						ShortName:   metricsUpdate.Items[0].GetShortName(),
					},
				},
			},
		}
		// rename a plugin
		scenario2 := testLeftJoinData{
			leftConfig:  metricsRenamed,
			rightConfig: metrics,
			expected: &controlv1.PluginArchive{
				Items: []*controlv1.PluginArchiveEntry{
					{
						Op:          controlv1.PatchOp_Rename,
						GatewayPath: metricsRenamed.Items[0].GetBinaryPath(),
						AgentPath:   metrics.Items[0].GetBinaryPath(),
						Module:      metrics.Items[0].GetModule(),
						OldDigest:   metrics.Items[0].GetDigest(),
						NewDigest:   metricsRenamed.Items[0].GetDigest(),
						ShortName:   "renamed",
					},
				},
			},
		}

		scenario3 := testLeftJoinData{
			leftConfig:  addlogging,
			rightConfig: metricsUpdate,
			expected: &controlv1.PluginArchive{
				Items: []*controlv1.PluginArchiveEntry{
					{
						Op:          controlv1.PatchOp_Update,
						GatewayPath: addlogging.Items[0].GetBinaryPath(),
						AgentPath:   metricsUpdate.Items[0].GetBinaryPath(),
						Module:      addlogging.Items[0].GetModule(),
						OldDigest:   metricsUpdate.Items[0].GetDigest(),
						NewDigest:   addlogging.Items[0].GetDigest(),
						ShortName:   addlogging.Items[0].GetShortName(),
					},
					{
						Op:          controlv1.PatchOp_Create,
						GatewayPath: addlogging.Items[1].GetBinaryPath(),
						AgentPath:   "",
						Module:      addlogging.Items[1].GetModule(),
						OldDigest:   "",
						NewDigest:   addlogging.Items[1].GetDigest(),
						ShortName:   addlogging.Items[1].GetShortName(),
					},
				},
			},
		}

		scenario4 := testLeftJoinData{
			leftConfig:  removeLogging,
			rightConfig: addlogging,
			expected: &controlv1.PluginArchive{
				Items: []*controlv1.PluginArchiveEntry{
					{
						Op:          controlv1.PatchOp_Remove,
						GatewayPath: "",
						AgentPath:   addlogging.Items[1].GetBinaryPath(),
						Module:      addlogging.Items[1].GetModule(),
						OldDigest:   addlogging.Items[1].GetDigest(),
						NewDigest:   "",
						ShortName:   addlogging.Items[1].GetShortName(),
					},
				},
			},
		}
		scenario5 := testLeftJoinData{
			leftConfig:  metricsRenamedModule,
			rightConfig: metrics,
			expected: &controlv1.PluginArchive{
				Items: []*controlv1.PluginArchiveEntry{
					{
						Op:          controlv1.PatchOp_Create,
						GatewayPath: metricsRenamedModule.Items[0].GetBinaryPath(),
						AgentPath:   "",
						Module:      metricsRenamedModule.Items[0].GetModule(),
						OldDigest:   "",
						NewDigest:   metricsRenamedModule.Items[0].GetDigest(),
						ShortName:   metricsRenamedModule.Items[0].GetShortName(),
					},
					{
						Op:          controlv1.PatchOp_Remove,
						GatewayPath: "",
						AgentPath:   metrics.Items[0].GetBinaryPath(),
						Module:      metrics.Items[0].GetModule(),
						OldDigest:   metrics.Items[0].GetDigest(),
						NewDigest:   "",
						ShortName:   metrics.Items[0].GetShortName(),
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
			for _, sc := range scenarios {
				ls := patch.LeftJoinOn(sc.leftConfig, sc.rightConfig)
				Expect(ls.Validate()).To(Succeed())
				Expect(len(ls.Items)).To(Equal(len(sc.expected.Items)))
				for i, data := range ls.Items {
					Expect(data.Op).To(Equal(sc.expected.Items[i].Op))
					Expect(data.AgentPath).To(Equal(sc.expected.Items[i].AgentPath))
					Expect(data.GatewayPath).To(Equal(sc.expected.Items[i].GatewayPath))
					Expect(data.NewDigest).To(Equal(sc.expected.Items[i].NewDigest))
					Expect(data.OldDigest).To(Equal(sc.expected.Items[i].OldDigest))
					Expect(data.Module).To(Equal(sc.expected.Items[i].Module))
					Expect(data.ShortName).To(Equal(sc.expected.Items[i].ShortName))
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
			Dirs: []string{tmpDir},
		}, test.Log)
		Expect(err).NotTo(HaveOccurred())

		mv1.Sort() // sort by module name
		Expect(mv1.Items).To(HaveLen(2))
		Expect(mv1.Items[0].Module).To(Equal(test1Module))
		Expect(mv1.Items[1].Module).To(Equal(test2Module))
		Expect(mv1.Items[0].Digest).To(Equal(v1Manifest.Items[0].Digest))
		Expect(mv1.Items[1].Digest).To(Equal(v1Manifest.Items[1].Digest))
		Expect(mv1.Items[0].BinaryPath).To(Equal(filepath.Join(tmpDir, "plugin_test1")))
		Expect(mv1.Items[1].BinaryPath).To(Equal(filepath.Join(tmpDir, "plugin_test2")))
		Expect(mv1.Items[0].GoVersion).To(Equal(runtime.Version()))
		Expect(mv1.Items[1].GoVersion).To(Equal(runtime.Version()))
		Expect(mv1.Items[0].ShortName).To(Equal("plugin_test1"))
		Expect(mv1.Items[1].ShortName).To(Equal("plugin_test2"))
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
				Dirs: []string{tmpDir},
			}, test.Log)
			Expect(err).NotTo(HaveOccurred())

			Expect(mv1.Items).To(HaveLen(1))
			Expect(mv1.Items[0].Module).To(Equal(test1Module))
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
				Dirs: []string{tmpDir},
			}, test.Log)

			Expect(err).NotTo(HaveOccurred())
			Expect(mv1.Items).To(HaveLen(1))

			Expect(mv1.Items[0].Module).To(Equal(test1Module))
		})
	})
})
