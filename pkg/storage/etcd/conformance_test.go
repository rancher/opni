package etcd_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/testruntime"

	"github.com/rancher/opni/pkg/storage/conformance"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/future"
)

func TestEtcd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcd Storage Suite")
}

var store = future.New[*etcd.EtcdStore]()

var _ = BeforeSuite(func() {
	testruntime.IfLabelFilterMatches(Label("integration", "slow"), func() {
		env := test.Environment{
			TestBin: "../../../testbin/bin",
		}
		env.Start(
			test.WithEnableGateway(false),
			test.WithEnableEtcd(true),
			test.WithEnableJetstream(false),
		)

		client, err := etcd.NewEtcdStore(context.Background(), env.EtcdConfig(),
			etcd.WithPrefix("test"),
		)
		Expect(err).NotTo(HaveOccurred())
		store.Set(client)

		DeferCleanup(env.Stop)
	})
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow"), conformance.TokenStoreTestSuite(store))
var _ = Describe("Cluster Store", Ordered, Label("integration", "slow"), conformance.ClusterStoreTestSuite(store))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow"), conformance.RBACStoreTestSuite(store))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow"), conformance.KeyringStoreTestSuite(store))
var _ = Describe("KV Store", Ordered, Label("integration", "slow"), conformance.KeyValueStoreTestSuite(store))
