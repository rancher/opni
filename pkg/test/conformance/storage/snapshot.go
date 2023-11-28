package conformance_storage

import (
	"context"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/snapshot"
	"github.com/rancher/opni/pkg/util/future"
)

func SnapshotSuiteTest[T snapshot.BackupRestore](
	implF future.Future[T],
) func() {
	return func() {
		var ctx context.Context
		var snapshotDir string
		var impl snapshot.BackupRestore
		BeforeAll(func() {
			dir, err := os.MkdirTemp("/tmp", "snapshots-*")
			Expect(err).To(Succeed())
			snapshotDir = dir
			ctx = context.TODO()
			impl = implF.Get()
			DeferCleanup(func() {
				os.RemoveAll(snapshotDir)
			})
		})

		AfterEach(func() {
			Expect(removeAllContents(snapshotDir))
		})

		Context("filesystem snapshots", func() {
			It("should snapshot/restore", func() {
				cfg := snapshot.SnapshotConfig{
					SnapshotDir:  snapshotDir,
					DataDir:      snapshotDir,
					SnapshotName: "my-snapshot",
				}
				Expect(cfg.Validate()).To(Succeed())

				m := snapshot.NewSnapshotManager(
					impl,
					cfg,
					logger.New(),
				)

				By("verifying the snapshots are successful")
				Expect(m.Save(ctx)).To(Succeed())

				items, err := m.List(ctx)
				Expect(err).To(Succeed())
				Expect(items).To(HaveLen(1))

				By("verifying the identifying snapshot metadata is correct")
				snapMd := items[0]
				Expect(snapMd.Name).To(HavePrefix("my-snapshot"))
				Expect(snapMd.Compressed).To(BeFalse())
				Expect(snapMd.Status).To(Equal(snapshot.SnapshotStatusSuccessful))
				finfo, err := os.Stat(strings.TrimPrefix(snapMd.Location, "file://"))
				Expect(err).To(Succeed())
				Expect(finfo.Size()).NotTo(Equal(0))
				Expect(finfo.Size()).To(Equal(snapMd.Size))

				By("verifying we can restore from the snapshot metadata")
				Expect(m.Restore(ctx, snapMd)).To(Succeed())
			})
			It("should snapshot/restore with compression", func() {
				cfg := snapshot.SnapshotConfig{
					SnapshotDir:  snapshotDir,
					DataDir:      snapshotDir,
					SnapshotName: "compr-snapshot",
					Compression: &snapshot.CompressionConfig{
						Type: snapshot.CompressionZip,
					},
				}
				Expect(cfg.Validate()).To(Succeed())

				m := snapshot.NewSnapshotManager(
					impl,
					cfg,
					logger.New(),
				)

				By("verifying the snapshots are successful")
				Expect(m.Save(ctx)).To(Succeed())

				items, err := m.List(ctx)
				Expect(err).To(Succeed())
				Expect(items).To(HaveLen(1))

				By("verifying the identifying snapshot metadata is correct")
				snapMd := items[0]
				Expect(snapMd.Name).To(HavePrefix("compr-snapshot"))
				Expect(snapMd.Compressed).To(BeTrue())
				Expect(snapMd.Status).To(Equal(snapshot.SnapshotStatusSuccessful))
				finfo, err := os.Stat(strings.TrimPrefix(snapMd.Location, "file://"))
				Expect(err).To(Succeed())
				Expect(finfo.Size()).NotTo(Equal(0))
				Expect(finfo.Size()).To(Equal(snapMd.Size))

				By("verifying we can restore from the snapshot metadata")
				Expect(m.Restore(ctx, snapMd)).To(Succeed())
			})

			It("should enforce the configured retention limit", func() {

			})
		})

		Context("s3 snapshots", func() {
			It("should backup/restore to/from s3", func() {

			})
			It("should backup/restore to/from s3 w/ compression", func() {

			})
			It("should enforce the configured retention policy", func() {

			})
		})

	}
}

func removeAllContents(dirPath string) error {
	// Read the contents of the directory
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}

	// Iterate through each entry and remove it
	for _, entry := range entries {
		entryPath := fmt.Sprintf("%s/%s", dirPath, entry.Name())

		// Remove files
		if entry.IsDir() {
			// Remove directories recursively
			err = os.RemoveAll(entryPath)
			if err != nil {
				return err
			}
		} else {
			err = os.Remove(entryPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
