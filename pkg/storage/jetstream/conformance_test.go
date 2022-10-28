package jetstream_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/storage/conformance"
	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/future"
)

func TestJetStream(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "JetStream Storage Suite")
}

var store = future.New[*jetstream.JetStreamStore]()

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
	}
	env.Start(
		test.WithEnableCortex(false),
		test.WithEnableGateway(false),
		test.WithEnableEtcd(false),
		test.WithEnableJetstream(true),
		test.WithEnableDisconnectServer(false),
		test.WithEnableRealtimeServer(false),
	)

	s, err := jetstream.NewJetStreamStore(context.Background(), env.JetStreamConfig())
	Expect(err).NotTo(HaveOccurred())
	store.Set(s)

	DeferCleanup(env.Stop)
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow"), conformance.TokenStoreTestSuite(store))
var _ = Describe("Cluster Store", Ordered, Label("integration", "slow"), conformance.ClusterStoreTestSuite(store))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow"), conformance.RBACStoreTestSuite(store))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow"), conformance.KeyringStoreTestSuite(store))
var _ = Describe("KV Store", Ordered, Label("integration", "slow"), conformance.KeyValueStoreTestSuite(store))
