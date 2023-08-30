package jetstream_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/test"
	. "github.com/rancher/opni/pkg/test/conformance/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util/future"
)

func TestJetStream(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "JetStream Storage Suite")
}

func init() {
	jetstream.DefaultLeaserExpireDuration = 1 * time.Second
}

var store = future.New[*jetstream.JetStreamStore]()
var lmF = future.New[*jetstream.JetStreamLockManager]()

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

		lm, err := jetstream.NewLockManager(context.Background(), env.JetStreamConfig())
		Expect(err).NotTo(HaveOccurred())
		lmF.Set(lm)

		DeferCleanup(env.Stop)
	})
})

// var _ = Describe("Token Store", Ordered, Label("integration", "slow"), TokenStoreTestSuite(store))
// var _ = Describe("Cluster Store", Ordered, Label("integration", "slow"), ClusterStoreTestSuite(store))
// var _ = Describe("RBAC Store", Ordered, Label("integration", "slow"), RBACStoreTestSuite(store))
// var _ = Describe("Keyring Store", Ordered, Label("integration", "slow"), KeyringStoreTestSuite(store))
// var _ = Describe("KV Store", Ordered, Label("integration", "slow"), KeyValueStoreTestSuite(store))
var _ = Describe("Lock manager", Ordered, Label("integration", "slow"), LockManagerTestSuite(lmF))
