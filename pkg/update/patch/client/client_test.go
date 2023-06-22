package client_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/memfs"
	"github.com/rancher/opni/pkg/test/testlog"
	"github.com/rancher/opni/pkg/update"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	controlv1 "github.com/rancher/opni/pkg/apis/control/v1"
	"github.com/rancher/opni/pkg/test/testutil"
	patchclient "github.com/rancher/opni/pkg/update/patch/client"
	"github.com/spf13/afero"
)

type optype = controlv1.PatchOp

const (
	create   optype = optype(controlv1.PatchOp_Create)
	opUpdate optype = optype(controlv1.PatchOp_Update)
	remove   optype = optype(controlv1.PatchOp_Remove)
	rename   optype = optype(controlv1.PatchOp_Rename)
	none     optype = optype(controlv1.PatchOp_None)
)

const (
	test1 = "test1"
	test2 = "test2"
)

const (
	v1 = "v1"
	v2 = "v2"
)

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
			Package:   testPackages[plugin],
			Data:      testBinaries[plugin][version],
			NewDigest: testBinaryDigests[plugin][version],
			Path:      plugin,
		}
	case opUpdate:
		return &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_Update,
			Package:   testPackages[plugin],
			Data:      testPatches[plugin].Bytes(),
			OldDigest: testBinaryDigests[plugin][version],
			NewDigest: testBinaryDigests[plugin][opts[0].(string)],
			Path:      plugin,
		}
	case remove:
		return &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_Remove,
			Package:   testPackages[plugin],
			OldDigest: testBinaryDigests[plugin][version],
			Path:      plugin,
		}
	case rename:
		return &controlv1.PatchSpec{
			Op:        controlv1.PatchOp_Rename,
			Package:   testPackages[plugin],
			OldDigest: testBinaryDigests[plugin][version],
			NewDigest: testBinaryDigests[plugin][version],
			Path:      opts[0].(string),
			Data:      []byte(opts[1].(string)),
		}
	}
	panic("invalid operation")
}

func newFs(mkdirs ...string) afero.Afero {
	fsys := afero.Afero{
		Fs: memfs.NewModeAwareMemFs(),
	}
	fsys.Chmod("/", 0o777)
	for _, dir := range mkdirs {
		if err := fsys.MkdirAll(dir, 0o777); err != nil {
			panic("test bug: " + err.Error())
		}
	}
	fsys.Mkdir("/tmp", 0o777)
	fsys.Mkdir("/tmp/testbin", 0o777)

	for name, v := range testBinaries {
		for version, contents := range v {
			if err := fsys.WriteFile(fmt.Sprintf("/tmp/testbin/%s%s", name, version), contents, 0o777); err != nil {
				panic("test bug: " + err.Error())
			}
		}
	}
	return afero.Afero{Fs: fsys}
}

var _ = Describe("Client", Label("unit"), func() {
	When("creating a new patch client", func() {
		It("should create the default plugin directory if it does not exist yet", func() {
			fsys := newFs("/plugins")
			pluginDir := "/plugins"

			_, err := patchclient.NewPatchClient(pluginDir, testlog.Log, patchclient.WithBaseFS(fsys))
			Expect(err).NotTo(HaveOccurred())

			_, err = fsys.Stat(pluginDir)
			Expect(err).NotTo(HaveOccurred())

			entries, err := fsys.ReadDir(pluginDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).To(BeEmpty())
		})
		When("no directories are specified", func() {
			It("should return an error", func() {
				pluginDir := ""
				_, err := patchclient.NewPatchClient(pluginDir, testlog.Log)
				Expect(err).To(HaveOccurred())
			})
		})
		When("the directory is not writable", func() {
			It("should return an error", func() {
				fs := newFs("/plugins")
				pluginDir := "/plugins/foo"

				fs.Chmod("/plugins", 0555)

				_, err := patchclient.NewPatchClient(pluginDir, testlog.Log, patchclient.WithBaseFS(fs))
				Expect(err).To(MatchError(ContainSubstring("failed to create")))
			})
		})
		When("the directory is not readable", func() {
			It("should return an error", func() {
				fs := newFs("/plugins")
				pluginDir := "/plugins/foo"

				fs.Chmod("/plugins", 0)

				_, err := patchclient.NewPatchClient(pluginDir, testlog.Log, patchclient.WithBaseFS(fs))
				Expect(err).To(MatchError(ContainSubstring("failed to stat")))
			})
		})
	})
	When("patching from a plugin archive", Ordered, func() {
		var pluginDir string
		var client update.SyncHandler
		var fsys afero.Afero
		BeforeAll(func() {
			fsys = newFs("/plugins")
			var err error
			pluginDir = "/plugins"
			client, err = patchclient.NewPatchClient(pluginDir, testlog.Log, patchclient.WithBaseFS(fsys))
			Expect(err).NotTo(HaveOccurred())
		})
		When("receiving a create operation", func() {
			It("should write the plugin to the default directory", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}
				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				_, err = fsys.Stat(pluginDir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := fsys.ReadDir(pluginDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(b2sum(fsys, path.Join(pluginDir, entries[0].Name()))).To(Equal(patches.Items[0].NewDigest))
			})
		})
		When("receiving an update operation", func() {
			It("should update the existing plugin with a patch", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2),
					},
				}
				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				_, err = fsys.Stat(pluginDir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := fsys.ReadDir(pluginDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(b2sum(fsys, path.Join(pluginDir, entries[0].Name()))).To(Equal(patches.Items[0].NewDigest))
			})
		})
		When("receiving a no-op operation", func() {
			It("should do nothing", func() {
				testStart := time.Now()
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:      controlv1.PatchOp_None,
							Package: test1Package,
							Path:    "test1",
						},
					},
				}
				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				entries, err := fsys.ReadDir(pluginDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(testutil.Must(entries[0].ModTime())).To(BeTemporally("<", testStart))
			})
		})
		When("receiving a rename operation", func() {
			It("should rename the plugin", func() {
				sum := testBinaryDigests[test1][v2]
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(rename, test1, v2, "test1", "renamed1"),
					},
				}
				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				_, err = fsys.Stat(pluginDir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := fsys.ReadDir(pluginDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("renamed1"))
				Expect(b2sum(fsys, path.Join(pluginDir, entries[0].Name()))).To(Equal(sum))
			})
		})
		When("receiving a remove operation", func() {
			It("should remove the plugin", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp_Remove,
							Package:   test1Package,
							OldDigest: testBinaryDigests[test1][v2],
							Path:      "renamed1",
						},
					},
				}
				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				_, err = fsys.Stat(pluginDir)
				Expect(err).NotTo(HaveOccurred())

				entries, err := fsys.ReadDir(pluginDir)
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				entries, err := fsys.ReadDir(pluginDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(2))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(entries[1].Name()).To(Equal("test2"))
				Expect(b2sum(fsys, path.Join(pluginDir, entries[0].Name()))).To(Equal(testBinaryDigests[test1][v1]))
				Expect(b2sum(fsys, path.Join(pluginDir, entries[1].Name()))).To(Equal(testBinaryDigests[test2][v1]))

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2),
						op(remove, test2, v1),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				entries, err = fsys.ReadDir(pluginDir)
				Expect(err).NotTo(HaveOccurred())
				Expect(entries).To(HaveLen(1))
				Expect(entries[0].Name()).To(Equal("test1"))
				Expect(b2sum(fsys, path.Join(pluginDir, entries[0].Name()))).To(Equal(testBinaryDigests[test1][v2]))
			})
		})
	})
	Context("error handling", func() {
		var pluginDir string
		var client update.SyncHandler
		var fsys afero.Afero
		BeforeEach(func() {
			fsys = newFs("/plugins")
			pluginDir = "/plugins"
			var err error
			client, err = patchclient.NewPatchClient(pluginDir, testlog.Log, patchclient.WithBaseFS(fsys))
			Expect(err).NotTo(HaveOccurred())
		})
		When("the default plugin directory is not writable", func() {
			It("should return an internal error", func() {
				fsys.Chmod("/plugins", 0o555)
				DeferCleanup(func() {
					fsys.Chmod("/plugins", 0o755)
				})

				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}
				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				fsys.Chmod(path.Join("/plugins", "test1"), 0o555)

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())
				// check the permissions
				fi, err := fsys.Stat(path.Join("/plugins", "test1"))
				Expect(err).NotTo(HaveOccurred())
				Expect(fi.Mode().Perm()).To(Equal(fs.FileMode(0o555)))
			})
		})
		When("attempting to patch a plugin that is not readable", func() {
			It("should return an internal error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				fsys.Chmod(path.Join("/plugins", "test1"), 0o333)

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2, func(e *controlv1.PatchSpec) {
							e.Data = testBinaries[test1][v2]
						}),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				fsys.Chmod("/plugins", 0o555)
				DeferCleanup(func() {
					fsys.Chmod("/plugins", 0o755)
				})

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(remove, test1, v1),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("could not remove plugin"))

				// Ensure the plugin is still there
				_, err = fsys.Stat(path.Join("/plugins", "test1"))
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				fsys.Chmod("/plugins", 0o555)
				DeferCleanup(func() {
					fsys.Chmod("/plugins", 0o755)
				})

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(rename, test1, v1, "test1", "renamed1"),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Internal))
				Expect(err.Error()).To(ContainSubstring("could not rename plugin"))

				// Ensure the old plugin is still there
				_, err = fsys.Stat(path.Join("/plugins", "test1"))
				Expect(err).NotTo(HaveOccurred())

				// Ensure the new plugin is not there
				_, err = fsys.Stat(path.Join("/plugins", "renamed1"))
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				// rename test2 to test1 (which already exists)

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(rename, test2, v1, "test2", "test1"),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(And(ContainSubstring("could not rename plugin"), ContainSubstring("already exists")))

				// Ensure the old plugins are still there
				_, err = fsys.Stat(path.Join("/plugins", "test2"))
				Expect(err).NotTo(HaveOccurred())

				_, err = fsys.Stat(path.Join("/plugins", "test1"))
				Expect(err).NotTo(HaveOccurred())

				Expect(b2sum(fsys, path.Join(pluginDir, "test1"))).To(Equal(testBinaryDigests[test1][v1]))
				Expect(b2sum(fsys, path.Join(pluginDir, "test2"))).To(Equal(testBinaryDigests[test2][v1]))
			})
		})
		When("an unknown patch operation is encountered", func() {
			It("should return an internal error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						{
							Op:        controlv1.PatchOp(999),
							Package:   test1Package,
							Data:      testBinaries[test1][v1],
							OldDigest: "",
							NewDigest: testBinaryDigests[test1][v1],
							Path:      "test1",
						},
					},
				}

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2),
						op(opUpdate, test2, v1, v2, func(e *controlv1.PatchSpec) {
							e.NewDigest = "foo"
						}),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(ContainSubstring("checksum mismatch"))

				// ensure test1 is updated
				Expect(b2sum(fsys, path.Join(pluginDir, "test1"))).To(Equal(testBinaryDigests[test1][v2]))

				// ensure test2 is unchanged
				Expect(b2sum(fsys, path.Join(pluginDir, "test2"))).To(Equal(testBinaryDigests[test2][v1]))

				arc3 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test2, v1, v2),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc3,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				// ensure test2 is updated
				Expect(b2sum(fsys, path.Join(pluginDir, "test2"))).To(Equal(testBinaryDigests[test2][v2]))
			})
		})
		When("updating a plugin and the old digest does not match", func() {
			It("should return an unavailable error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				// change something in the binary
				f, err := fsys.OpenFile(filepath.Join("/plugins", patches.Items[0].Path), os.O_WRONLY|os.O_APPEND, 0)
				Expect(err).NotTo(HaveOccurred())
				_, err = f.WriteString("foo")
				Expect(err).NotTo(HaveOccurred())
				Expect(f.Close()).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
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

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).NotTo(HaveOccurred())

				// delete the file
				Expect(fsys.Remove(filepath.Join("/plugins", patches.Items[0].Path))).NotTo(HaveOccurred())

				arc2 := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(opUpdate, test1, v1, v2),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(And(ContainSubstring("failed to stat plugin")))
			})
		})
		When("updating a plugin with a malformed patch", func() {
			It("should return an unavailable error", func() {
				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}

				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				err := client.HandleSyncResults(context.Background(), syncResp)
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
						op(opUpdate, test1, v1, v2, func(e *controlv1.PatchSpec) {
							e.Data = corruptPatch
						}),
					},
				}

				syncResp = &controlv1.SyncResults{
					RequiredPatches: arc2,
				}
				err = client.HandleSyncResults(context.Background(), syncResp)
				Expect(err).To(HaveOccurred())
				Expect(status.Code(err)).To(Equal(codes.Unavailable))
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("failed applying patch for plugin %s: corrupt patch", test1Package)))
			})
		})
		When("the plugin directory is located on a separate filesystem than the os temp directory", func() {
			It("should not use the rename strategy to replace plugins", func() {
				testfs := &memfs.CrossDeviceTestFs{Fs: fsys}
				client, err := patchclient.NewPatchClient(pluginDir, testlog.Log, patchclient.WithBaseFS(testfs))
				Expect(err).NotTo(HaveOccurred())

				patches := &controlv1.PatchList{
					Items: []*controlv1.PatchSpec{
						op(create, test1, v1),
					},
				}
				syncResp := &controlv1.SyncResults{
					RequiredPatches: patches,
				}
				Expect(client.HandleSyncResults(context.Background(), syncResp)).To(Succeed())
			})
		})
	})
})
