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
var lockManager = future.New[*etcd.EtcdLockManager]()

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

		DeferCleanup(env.Stop, "Test Suite Finished")
	})
})

var _ = Describe("Etcd Token Store", Ordered, Label("integration", "slow"), TokenStoreTestSuite(store))
var _ = Describe("Etcd Cluster Store", Ordered, Label("integration", "slow"), ClusterStoreTestSuite(store))
var _ = Describe("Etcd RBAC Store", Ordered, Label("integration", "slow"), RBACStoreTestSuite(store))
var _ = Describe("Etcd Keyring Store", Ordered, Label("integration", "slow"), KeyringStoreTestSuite(store))
var _ = Describe("Etcd KV Store", Ordered, Label("integration", "slow"), KeyValueStoreTestSuite(store, NewBytes, Equal))
var _ = Describe("Etcd Lock Manager", Ordered, Label("integration", "slow"), LockManagerTestSuite(store))
