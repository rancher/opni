package snapshot_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/snapshot"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testruntime"
)

type testSnapshotter struct {
	contents []byte
}

var _ snapshot.Snapshotter = (*testSnapshotter)(nil)

func (t *testSnapshotter) Save(_ context.Context, path string) error {
	return os.WriteFile(path, t.contents, 0700)
}

func (t *testSnapshotter) Restore(_ context.Context, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	t.contents = data
	return nil
}

func (t *testSnapshotter) data() []byte {
	return t.contents
}

func newTestSnapshotter(contents []byte) *testSnapshotter {
	return &testSnapshotter{}
}

var _ = Describe("Snapshot Manager", Ordered, Label("unit"), func() {
	var ctx context.Context
	var snapshotDir string
	BeforeAll(func() {
		dir, err := os.MkdirTemp("/tmp", "snapshots-*")
		Expect(err).To(Succeed())
		snapshotDir = dir
		ctx = context.TODO()
		DeferCleanup(func() {
			os.RemoveAll(snapshotDir)
		})
	})

	AfterEach(func() {
		Expect(removeAllContents(snapshotDir)).To(Succeed())
	})

	It("should create local snapshots", func() {
		cfg := snapshot.SnapshotConfig{
			SnapshotDir:  snapshotDir,
			DataDir:      snapshotDir,
			SnapshotName: "my-snapshot",
		}
		Expect(cfg.Validate()).To(Succeed())

		s := newTestSnapshotter([]byte("data"))
		backedData := []byte{}
		copy(backedData, s.data())

		m := snapshot.NewSnapshotManager(
			s,
			cfg,
			logger.New(),
		)

		By("verifying the snapshots are successful")
		Expect(m.Save(ctx)).To(Succeed())

		items, err := m.List(ctx)
		Expect(err).To(Succeed())
		Expect(items).To(HaveLen(1))

		By("veryfying the identifying snapshot metadata is correct")
		snapMd := items[0]
		Expect(snapMd.Name).To(HavePrefix("my-snapshot"))
		Expect(snapMd.Compressed).To(BeFalse())
		Expect(snapMd.Status).To(Equal(snapshot.SnapshotStatusSuccessful))
		finfo, err := os.Stat(strings.TrimPrefix(snapMd.Location, "file://"))
		Expect(err).To(Succeed())
		Expect(finfo.Size()).NotTo(Equal(0))
		Expect(finfo.Size()).To(Equal(snapMd.Size))

		By("verifying we can restore from the snapshot metadata")
		s.contents = []byte("newer data")
		Expect(m.Restore(ctx, snapMd)).To(Succeed())
		Expect(s.data()).To(Equal(backedData))
	})

	It("should create compressed (zip) local snapshots", func() {
		cfg := snapshot.SnapshotConfig{
			SnapshotDir:  snapshotDir,
			DataDir:      snapshotDir,
			SnapshotName: "compr-snapshot",
			Compression: &snapshot.CompressionConfig{
				Type: snapshot.CompressionZip,
			},
		}
		Expect(cfg.Validate()).To(Succeed())

		uncompressedSize := 10000
		s := newTestSnapshotter(simpleTestData(uncompressedSize))
		backedData := []byte{}
		copy(backedData, s.data())

		m := snapshot.NewSnapshotManager(
			s,
			cfg,
			logger.New(),
		)
		By("verifying that compressed snapshots are successful")
		Expect(m.Save(ctx)).To(Succeed())

		items, err := m.List(ctx)
		Expect(err).To(Succeed())
		Expect(items).To(HaveLen(1))

		By("verifying the snapshot has the correct identifying metadata")
		snapMd := items[0]
		Expect(snapMd.Name).To(HavePrefix("compr-snapshot"))
		Expect(snapMd.Compressed).To(BeTrue())
		Expect(snapMd.Status).To(Equal(snapshot.SnapshotStatusSuccessful))
		finfo, err := os.Stat(strings.TrimPrefix(snapMd.Location, "file://"))
		Expect(err).To(Succeed())
		Expect(finfo.Size()).To(BeNumerically("<", uncompressedSize))

		By("verifying we can restore from a compressed snapshot")
		s.contents = []byte("newer data")
		Expect(m.Restore(ctx, snapMd)).To(Succeed())
		Expect(s.data()).To(Equal(backedData))
	})

	It("should be able to upload snapshots to S3", func() {
		testruntime.IfCI(
			func() {
				Skip("Skipping S3 uploads in CI")
			},
		)
		accessKeyId := os.Getenv("AWS_ACCESS_KEY_ID")
		secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
		bucket := os.Getenv("AWS_SNAPSHOT_BUCKET")
		if accessKeyId == "" || secretAccessKey == "" || bucket == "" {
			Fail("Insufficient config for running S3 suite")
		}

		s := newTestSnapshotter(
			[]byte("hello from remote"),
		)

		m := snapshot.NewSnapshotManager(
			s,
			snapshot.SnapshotConfig{
				SnapshotDir:  snapshotDir,
				SnapshotName: "remote-backup",
				Retention:    0,
				S3: &snapshot.S3Config{
					Endpoint:   "s3.us-east-2.amazonaws.com",
					AccessKey:  accessKeyId,
					SecretKey:  secretAccessKey,
					Region:     "us-east-2",
					Folder:     "snapshots",
					BucketName: bucket,
				},
			},
			logger.New(),
		)

		Expect(m.Save(ctx)).To(Succeed())
		items, err := m.List(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(items).To(HaveLen(1))

		snapMd := items[0]
		msg, err := base64.StdEncoding.DecodeString(snapMd.Message)
		Expect(err).To(Succeed())
		Expect(snapMd.Status).To(Equal(snapshot.SnapshotStatusSuccessful), string(msg))
	})
})

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

func simpleTestData(n int) []byte {
	ret := make([]byte, n)
	for i := 0; i < len(ret); i++ {
		ret[i] = 'a'
	}
	return ret
}
