package etcd_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
var errCtrl = future.New[conformance.ErrorController]()

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
	}
	env.Start(test.WithEnableCortex(false), test.WithEnableGateway(false))
	proc := env.Processes.Etcd.Get()

	timeout := 500 * time.Millisecond
	store.Set(etcd.NewEtcdStore(context.Background(), env.EtcdConfig(),
		etcd.WithCommandTimeout(timeout),
		etcd.WithPrefix("test"),
	))

	errCtrl.Set(conformance.NewProcessErrorController(proc))
	DeferCleanup(env.Stop)
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow", "temporal"), conformance.TokenStoreTestSuite(store, errCtrl))
var _ = Describe("Cluster Store", Ordered, Label("integration", "slow", "temporal"), conformance.ClusterStoreTestSuite(store, errCtrl))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow", "temporal"), conformance.RBACStoreTestSuite(store, errCtrl))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow", "temporal"), conformance.KeyringStoreTestSuite(store, errCtrl))
var _ = Describe("KV Store", Ordered, Label("integration", "slow", "temporal"), conformance.KeyValueStoreTestSuite(store, errCtrl))
