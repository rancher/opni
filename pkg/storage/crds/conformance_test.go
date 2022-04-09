package crds_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni-monitoring/pkg/storage/conformance"
	"github.com/rancher/opni-monitoring/pkg/storage/crds"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/util"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRDs Storage Suite")
}

var store = util.NewFuture[*crds.CRDStore]()
var errCtrl = util.NewFuture[conformance.ErrorController]()

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
		CRDDirectoryPaths: []string{
			"../../../deploy/charts/opni-monitoring/crds",
		},
		Logger: test.Log,
	}
	config, err := env.StartK8s()
	Expect(err).NotTo(HaveOccurred())

	store.Set(crds.NewCRDStore(crds.WithRestConfig(config), crds.WithCommandTimeout(100*time.Millisecond)))
	errCtrl.Set(conformance.NewProcessErrorController(env.Processes.APIServer.Get()))

	DeferCleanup(env.Stop)
})

var _ = Describe("Token Store", Ordered, conformance.TokenStoreTestSuite(store, errCtrl))
var _ = Describe("Cluster Store", Ordered, conformance.ClusterStoreTestSuite(store, errCtrl))
var _ = Describe("RBAC Store", Ordered, conformance.RBACStoreTestSuite(store, errCtrl))
