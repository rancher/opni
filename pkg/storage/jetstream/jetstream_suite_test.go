package jetstream_test

import (
	"context"
	"testing"

	"github.com/nats-io/jsm.go"
	"github.com/nats-io/nats.go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/test"
	. "github.com/rancher/opni/pkg/test/conformance/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/samber/lo"
)

func TestJetStream(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "JetStream Storage Suite")
}

var lmF = future.New[storage.LockManager]()
var lmSetF = future.New[lo.Tuple3[
	storage.LockManager, storage.LockManager, storage.LockManager,
]]()
var store = future.New[*jetstream.JetStreamStore]()

var snapshotter = future.New[*jetstream.Snapshotter]()

var _ = BeforeSuite(func() {
	testruntime.IfIntegration(func() {
		env := test.Environment{}
		env.Start(
			test.WithEnableGateway(false),
			test.WithEnableEtcd(false),
			test.WithEnableJetstream(true),
		)
		s, err := jetstream.NewJetStreamStore(context.Background(), env.JetStreamConfig())
		Expect(err).NotTo(HaveOccurred())
		store.Set(s)

		nc, js, err := jetstream.AcquireJetstreamConn(
			context.Background(),
			env.JetStreamConfig(),
			logger.NewNop(),
		)

		lm := jetstream.NewLockManager(
			context.Background(),
			js,
			"test",
			logger.New().WithGroup("js-lock"),
		)
		Expect(err).NotTo(HaveOccurred())
		lmF.Set(lm)

		_, js1, err := jetstream.AcquireJetstreamConn(
			context.Background(),
			env.JetStreamConfig(),
			logger.NewNop(),
		)
		Expect(err).NotTo(HaveOccurred())

		_, js2, err := jetstream.AcquireJetstreamConn(
			context.Background(),
			env.JetStreamConfig(),
			logger.NewNop(),
		)
		Expect(err).NotTo(HaveOccurred())

		_, js3, err := jetstream.AcquireJetstreamConn(
			context.Background(),
			env.JetStreamConfig(),
			logger.New(),
		)
		Expect(err).NotTo(HaveOccurred())

		x := jetstream.NewLockManager(context.Background(), js1, "test", logger.New())
		y := jetstream.NewLockManager(context.Background(), js2, "test", logger.New())
		z := jetstream.NewLockManager(context.Background(), js3, "test", logger.New())

		lmSetF.Set(lo.Tuple3[storage.LockManager, storage.LockManager, storage.LockManager]{
			A: x, B: y, C: z,
		})
		mgr, err := jsm.New(nc)
		Expect(err).NotTo(HaveOccurred())

		sn := jetstream.NewSnapshotter(mgr, logger.New())
		snapshotter.Set(sn)

		DeferCleanup(env.Stop, "Test Suite Finished")
	})
})

var _ = Describe("Jetstream Token Store", Ordered, Label("integration", "slow"), TokenStoreTestSuite(store))
var _ = Describe("Jetstream Cluster Store", Ordered, Label("integration", "slow"), ClusterStoreTestSuite(store))
var _ = Describe("Jetstream RBAC Store", Ordered, Label("integration", "slow"), RBACStoreTestSuite(store))
var _ = Describe("Jetstream Keyring Store", Ordered, Label("integration", "slow"), KeyringStoreTestSuite(store))
var _ = Describe("Jetstream KV Store", Ordered, Label("integration", "slow"), KeyValueStoreTestSuite(store, NewBytes, Equal))

var _ = Describe("Jetstream Lock Manager", Ordered, Label("integration", "slow"), LockManagerTestSuite(lmF, lmSetF))
var _ = Describe("Jetstream Backup Restore", Ordered, Label("integration", "slow"), SnapshotSuiteTest(snapshotter))

var _ = Context("Error Codes", func() {
	Specify("Nats KeyNotFound errors should be equal to ErrNotFound", func() {
		Expect(storage.IsNotFound(jetstream.JetstreamGrpcError(nats.ErrKeyNotFound))).To(BeTrue())
	})
	Specify("Nats KeyExists errors should be equal to ErrAlreadyExists", func() {
		Expect(storage.IsAlreadyExists(jetstream.JetstreamGrpcError(nats.ErrKeyExists))).To(BeTrue())
	})
	Specify("Nats KV sequence errors shoud be comparable with IsConflict", func() {
		Expect(storage.IsConflict(jetstream.JetstreamGrpcError(&nats.APIError{
			Code:        400,
			ErrorCode:   nats.JSErrCodeStreamWrongLastSequence,
			Description: "wrong last sequence: 1234",
		}))).To(BeTrue())
	})
})
