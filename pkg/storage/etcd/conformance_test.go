package etcd_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/storage/conformance"
	"github.com/rancher/opni-monitoring/pkg/storage/etcd"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/util"
)

func TestEtcd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcd Storage Suite")
}

var store = util.NewFuture[*etcd.EtcdStore]()
var errCtrl = util.NewFuture[conformance.ErrorController]()

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
		Logger:  test.Log,
	}
	env.Start(test.WithEnableCortex(false), test.WithEnableGateway(false))
	client, err := env.EtcdClient()
	Expect(err).NotTo(HaveOccurred())

	proc := env.Processes.Etcd.Get()

	timeout := 50 * time.Millisecond
	store.Set(&etcd.EtcdStore{
		EtcdStoreOptions: etcd.EtcdStoreOptions{
			CommandTimeout: timeout,
		},
		Client: client,
		Logger: test.Log.Named("etcd"),
	})

	errCtrl.Set(conformance.NewProcessErrorController(proc))
	DeferCleanup(env.Stop)
})

var _ = Describe("Token Store", Ordered, conformance.TokenStoreTestSuite(store, errCtrl))
var _ = Describe("Cluster Store", Ordered, conformance.ClusterStoreTestSuite(store, errCtrl))
var _ = Describe("RBAC Store", Ordered, conformance.RBACStoreTestSuite(store, errCtrl))
