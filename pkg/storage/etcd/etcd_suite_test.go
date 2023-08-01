package etcd_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/test"
	. "github.com/rancher/opni/pkg/test/conformance/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util/future"
)

func TestEtcd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcd Storage Suite")
}

var store = future.New[*etcd.EtcdStore]()

var lmsF = future.New[[]*etcd.EtcdLockManager]()

var _ = BeforeSuite(func() {
	testruntime.IfIntegration(func() {
		env := test.Environment{}
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
		lms := make([]*etcd.EtcdLockManager, 7)
		for i := 0; i < 2; i++ {
			l, err := etcd.NewEtcdLockManager(context.Background(), env.EtcdConfig(),
				etcd.WithPrefix("test"),
			)
			Expect(err).NotTo(HaveOccurred())
			lms[i] = l
		}

		lmsF.Set(lms)
		DeferCleanup(env.Stop)
	})
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow"), TokenStoreTestSuite(store))
var _ = Describe("Cluster Store", Ordered, Label("integration", "slow"), ClusterStoreTestSuite(store))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow"), RBACStoreTestSuite(store))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow"), KeyringStoreTestSuite(store))
var _ = Describe("KV Store", Ordered, Label("integration", "slow"), KeyValueStoreTestSuite(store))
var _ = Describe("Lock Manager", Ordered, Label("integration", "slow"), LockManagerTestSuite(lmsF))
