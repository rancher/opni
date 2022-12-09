package patch_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/patch"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/test/testutil"
)

type optype = controlv1.PatchOp

const (
	create optype = optype(controlv1.PatchOp_Create)
	update optype = optype(controlv1.PatchOp_Update)
	remove optype = optype(controlv1.PatchOp_Remove)
	rename optype = optype(controlv1.PatchOp_Rename)
	none   optype = optype(controlv1.PatchOp_None)
)

const (
	test1 = "test1"
	test2 = "test2"
)

const (
	v1 = "v1"
	v2 = "v2"
)

var defaultDir = new(string)

type editFunc = func(*controlv1.PatchSpec)

func op(ot optype, plugin string, version string, opts ...any) (retVal *controlv1.PatchSpec) {
	defer func() {
		for _, opt := range opts {
			if ef, ok := opt.(editFunc); ok {
				ef(retVal)
			}
		}
	}()
	switch ot {
	case create:
		return &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_Create,
			Module:    testModules[plugin],
			Data:      testutil.Must(os.ReadFile(*testBinaries[plugin][version])),
			NewDigest: b2sum(*testBinaries[plugin][version]),
			Filename:  plugin,
		}
	case update:
		return &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_Update,
			Module:    testModules[plugin],
			Data:      testPatches[plugin].Bytes(),
			OldDigest: b2sum(*testBinaries[plugin][version]),
			NewDigest: b2sum(*testBinaries[plugin][opts[0].(string)]),
			Filename:  plugin,
		}
	case remove:
		return &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_Remove,
			Module:    testModules[plugin],
			OldDigest: b2sum(*testBinaries[plugin][version]),
			Filename:  plugin,
		}
	case rename:
		return &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_Rename,
			Module:    testModules[plugin],
			OldDigest: b2sum(*testBinaries[plugin][version]),
			NewDigest: b2sum(*testBinaries[plugin][version]),
			Filename:  opts[0].(string),
			Data:      []byte(opts[1].(string)),
		}
	}
	panic("invalid operation")
}

var _ = Describe("Client", func() {
	When("creating a new patch client", func() {
		It("should create the default plugin directory if it does not exist yet", func() {
			tempDir, err := os.MkdirTemp("", "opni-pkg-cache-test")
			Expect(err).NotTo(HaveOccurred())
			conf := v1beta1.PluginsSpec{
				Dir: path.Join(tempDir, "plugins"),
			}
			DeferCleanup(func() {
				os.RemoveAll(tempDir)
			})

			_, err = patch.NewPatchClient(conf, test.Log)
			Expect(err).NotTo(HaveOccurred())

			_, err = os.Stat(conf.Dir)
			Expect(err).NotTo(HaveOccurred())

			entries, err := os.ReadDir(conf.Dir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})
		When("no directories are specified", func() {
			It("should return an error", func() {
				conf := v1beta1.PluginsSpec{}
				_, err := patch.NewPatchClient(conf, test.Log)
				Expect(err).To(HaveOccurred())
			})
		})
		When("the directory is not writable", func() {
			It("should return an error", func() {
				tempDir, err := os.MkdirTemp("", "opni-pkg-cache-test")
				Expect(err).NotTo(HaveOccurred())
				conf := v1beta1.PluginsSpec{
					Dir: path.Join(tempDir, "plugins"),
				}
				DeferCleanup(func() {
					Expect(os.Chmod(tempDir, 0o755)).To(Succeed())
					Expect(os.RemoveAll(tempDir)).To(Succeed())
				})

				err = os.Chmod(tempDir, 0o555)
				Expect(err).NotTo(HaveOccurred())

				_, err = patch.NewPatchClient(conf, test.Log)
				Expect(err).To(MatchError(ContainSubstring("failed to create")))
			})
		})
		When("the directory is not readable", func() {
			It("should return an error", func() {
				tempDir, err := os.MkdirTemp("", "opni-pkg-cache-test")
				Expect(err).NotTo(HaveOccurred())
				conf := v1beta1.PluginsSpec{
					Dir: path.Join(tempDir, "plugins"),
				}
				DeferCleanup(func() {
					Expect(os.Chmod(tempDir, 0o755)).To(Succeed())
					Expect(os.RemoveAll(tempDir)).To(Succeed())
				})

				err = os.Chmod(tempDir, 0)
				Expect(err).NotTo(HaveOccurred())

				_, err = patch.NewPatchClient(conf, test.Log)
				Expect(err).To(MatchError(ContainSubstring("failed to stat")))
			})
		})
	})
	When("patching from a plugin archive", Ordered, func() {
		var conf v1beta1.PluginsSpec
		var client patch.PatchClient
		BeforeAll(func() {
			tempDir, err := os.MkdirTemp("", "opni-pkg-cache-test")
			Expect(err).NotTo(HaveOccurred())
			conf = v1beta1.PluginsSpec{
				Dir: path.Join(tempDir, "plugins"),
			}
			DeferCleanup(func() {
				os.RemoveAll(tempDir)
			})

			client, err = patch.NewPatchClient(conf, test.Log)
			Expect(err).NotTo(HaveOccurred())

			*defaultDir = conf.Dir
		})
		When("receiving a create operation", func() {
			It("should write the plugin to the default directory", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}
				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(conf.Dir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := os.ReadDir(conf.Dir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(b2sum(path.Join(conf.Dir, entries[0].Name()))).To(Equal(patches.Items[0].NewDigest))
			})
		})
		When("receiving an update operation", func() {
			It("should update the existing plugin with a patch", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2),
					},
				}
				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(conf.Dir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := os.ReadDir(conf.Dir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(b2sum(path.Join(conf.Dir, entries[0].Name()))).To(Equal(patches.Items[0].NewDigest))
			})
		})
		When("receiving a no-op operation", func() {
			It("should do nothing", func() {
				testStart := time.Now()
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:       controlv1.PatchOp_None,
							Module:   test1Module,
							Filename: "test1",
						},
					},
				}
				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				entries, err := os.ReadDir(conf.Dir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(testutil.Must(entries[0].Info()).ModTime()).To(BeTemporally("<", testStart))
			})
		})
		When("receiving a rename operation", func() {
			It("should rename the plugin", func() {
				sum := b2sum(*test1v2BinaryPath)
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(rename, test1, v2, "test1", "renamed1"),
					},
				}
				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(conf.Dir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := os.ReadDir(conf.Dir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("renamed1"))
				Expect(b2sum(path.Join(conf.Dir, entries[0].Name()))).To(Equal(sum))
			})
		})
		When("receiving a remove operation", func() {
			It("should remove the plugin", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp_Remove,
							Module:    test1Module,
							OldDigest: b2sum(*test1v2BinaryPath),
							Filename:  "renamed1",
						},
					},
				}
				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(conf.Dir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := os.ReadDir(conf.Dir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(0))
			})
		})
		When("receiving multiple operations", func() {
			It("should apply them all", func() {
				// add v1 for both test1 and test2, then apply a patch to update test1
				// and remove test2

				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
						op(create, test2, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				entries, err := os.ReadDir(conf.Dir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(2))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(entries[1].Name()).To(Equal("test2"))
				Expect(b2sum(path.Join(conf.Dir, entries[0].Name()))).To(Equal(b2sum(*test1v1BinaryPath)))
				Expect(b2sum(path.Join(conf.Dir, entries[1].Name()))).To(Equal(b2sum(*test2v1BinaryPath)))

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2),
						op(remove, test2, v1),
					},
				}

				err = client.Patch(arc2)
				Expect(err).NotTo(HaveOccurred())

				entries, err = os.ReadDir(conf.Dir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(b2sum(path.Join(conf.Dir, entries[0].Name()))).To(Equal(b2sum(*test1v2BinaryPath)))
			})
		})
	})
	Context("error handling", func() {
		var conf v1beta1.PluginsSpec
		var client patch.PatchClient
		BeforeEach(func() {
			tempDir, err := os.MkdirTemp("", "opni-pkg-cache-test")
			Expect(err).NotTo(HaveOccurred())
			conf = v1beta1.PluginsSpec{
				Dir: path.Join(tempDir, "plugins"),
			}
			DeferCleanup(func() {
				os.RemoveAll(tempDir)
			})

			client, err = patch.NewPatchClient(conf, test.Log)
			Expect(err).NotTo(HaveOccurred())

			*defaultDir = conf.Dir
		})
		When("the default plugin directory is not writable", func() {
			It("should return an internal error", func() {
				os.Chmod(*defaultDir, 0o555)
				DeferCleanup(func() {
					os.Chmod(*defaultDir, 0o755)
				})

				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("could not write plugin"))
			})
		})
		When("attempting to patch a plugin that is not writable", func() {
			It("should succeed if the directory is writable", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				os.Chmod(path.Join(*defaultDir, "test1"), 0o555)

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2),
					},
				}

				err = client.Patch(arc2)
				Expect(err).NotTo(HaveOccurred())
				// check the permissions
				fi, err := os.Stat(path.Join(*defaultDir, "test1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(fi.Mode().Perm()).To(Equal(os.FileMode(0o755)))
			})
		})
		When("attempting to patch a plugin that is not readable", func() {
			It("should return an internal error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				os.Chmod(path.Join(*defaultDir, "test1"), 0o333)

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("failed to read plugin"))
			})
		})
		When("the server sends a malformed patch", func() {
			It("should return an internal error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2, func(e *controlv1.PatchSpec) {
							e.Data = testutil.Must(os.ReadFile(*test1v2BinaryPath))
						}),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("unknown patch format"))
			})
		})
		When("attempting to remove a non-existent plugin", func() {
			It("should do nothing and not return an error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(remove, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("a permission error is encountered while removing a plugin", func() {
			It("should return an internal error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				os.Chmod(*defaultDir, 0o555)
				DeferCleanup(func() {
					os.Chmod(*defaultDir, 0o755)
				})

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(remove, test1, v1),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("could not remove plugin"))

				// Ensure the plugin is still there
				_, err = os.Stat(path.Join(*defaultDir, "test1"))
				Expect(err).NotTo(HaveOccurred())
			})
		})
		When("a non-permission error is encountered while removing a plugin", func() {
			It("should return an unavailable error", func() {
				// todo: how to test this?
			})
		})
		When("a permission error is encountered while renaming a plugin", func() {
			It("should return an internal error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				os.Chmod(*defaultDir, 0o555)
				DeferCleanup(func() {
					os.Chmod(*defaultDir, 0o755)
				})

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(rename, test1, v1, "test1", "renamed1"),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("could not rename plugin"))

				// Ensure the old plugin is still there
				_, err = os.Stat(path.Join(*defaultDir, "test1"))
				Expect(err).NotTo(HaveOccurred())

				// Ensure the new plugin is not there
				_, err = os.Stat(path.Join(*defaultDir, "renamed1"))
				Expect(err).To(HaveOccurred())
				Expect(os.IsNotExist(err)).To(BeTrue())
			})
		})
		When("a non-permission error is encountered while renaming a plugin", func() {
			It("should return an unavailable error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
						op(create, test2, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				// rename test2 to test1 (which already exists)

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(rename, test2, v1, "test2", "test1"),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(And(ContainSubstring("could not rename plugin"), ContainSubstring("already exists")))

				// Ensure the old plugins are still there
				_, err = os.Stat(path.Join(*defaultDir, "test2"))
				Expect(err).NotTo(HaveOccurred())

				_, err = os.Stat(path.Join(*defaultDir, "test1"))
				Expect(err).NotTo(HaveOccurred())

				Expect(b2sum(path.Join(conf.Dir, "test1"))).To(Equal(b2sum(*test1v1BinaryPath)))
				Expect(b2sum(path.Join(conf.Dir, "test2"))).To(Equal(b2sum(*test2v1BinaryPath)))
			})
		})
		When("an unknown patch operation is encountered", func() {
			It("should return an internal error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp(999),
							Module:    test1Module,
							Data:      testutil.Must(os.ReadFile(*test1v1BinaryPath)),
							OldDigest: "",
							NewDigest: b2sum(*test1v1BinaryPath),
							Filename:  "test1",
						},
					},
				}

				err := client.Patch(patches)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("unknown patch operation 999"))
			})
		})
		When("one patch operation succeeds but one fails", func() {
			It("should return an overall error and preserve the successful operations", func() {
				// 1. create test1v1 and test2v1
				// 2. patch test1v1 to test1v2 and test2v1 to test2v2, but the checksum post-patch for test2 does not match
				// 3. ensure test1v2 is present and test2v1 is present/unchanged
				// 4. patch again, this time with the correct checksum for test2v2
				// 5. ensure test1v2 is present and test2v2 is present and contains the correct data

				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
						op(create, test2, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2),
						op(update, test2, v1, v2, func(e *controlv1.PatchSpec) {
							e.NewDigest = "foo"
						}),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(ContainSubstring("checksum mismatch"))

				// ensure test1 is updated
				Expect(b2sum(path.Join(conf.Dir, "test1"))).To(Equal(b2sum(*test1v2BinaryPath)))

				// ensure test2 is unchanged
				Expect(b2sum(path.Join(conf.Dir, "test2"))).To(Equal(b2sum(*test2v1BinaryPath)))

				arc3 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test2, v1, v2),
					},
				}

				err = client.Patch(arc3)
				Expect(err).NotTo(HaveOccurred())

				// ensure test2 is updated
				Expect(b2sum(path.Join(conf.Dir, "test2"))).To(Equal(b2sum(*test2v2BinaryPath)))
			})
		})
		When("updating a plugin and the old digest does not match", func() {
			It("should return an unavailable error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				// change something in the binary
				f, err := os.OpenFile(filepath.Join(*defaultDir, patches.Items[0].Filename), os.O_WRONLY|os.O_APPEND, 0)
				Expect(err).NotTo(HaveOccurred())
				_, err = f.WriteString("foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(f.Close()).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(And(ContainSubstring("existing plugin"), ContainSubstring("is invalid, cannot apply patch")))
			})
		})
		When("updating a plugin and the old file does not exist", func() {
			It("should return an unavailable error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				// delete the file
				Expect(os.Remove(filepath.Join(*defaultDir, patches.Items[0].Filename))).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(And(ContainSubstring("failed to stat plugin"), ContainSubstring("no such file or directory")))
			})
		})
		When("updating a plugin with a malformed patch", func() {
			It("should return an unavailable error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				err := client.Patch(patches)
				Expect(err).NotTo(HaveOccurred())

				// https://github.com/gabstv/go-bsdiff/blob/master/pkg/bspatch/bspatch_test.go#L226
				corruptPatch := []byte{
					0x42, 0x53, 0x44, 0x49, 0x46, 0x46, 0x34, 0x30,
					0x64, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80,
					0x2A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
					0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				}

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(update, test1, v1, v2, func(e *controlv1.PatchSpec) {
							e.Data = corruptPatch
						}),
					},
				}

				err = client.Patch(arc2)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("failed applying patch for plugin %s: corrupt patch", test1Module)))
			})
		})
	})
})
