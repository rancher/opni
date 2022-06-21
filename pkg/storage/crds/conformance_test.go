package crds_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage/conformance"
	"github.com/rancher/opni/pkg/storage/crds"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/future"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRDs Storage Suite")
}

var store = future.New[*crds.CRDStore]()
var errCtrl = future.New[conformance.ErrorController]()

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
		CRDDirectoryPaths: []string{
			"../../../config/crd/bases",
		},
	}
	config, err := env.StartK8s()
	Expect(err).NotTo(HaveOccurred())

	store.Set(crds.NewCRDStore(crds.WithRestConfig(config), crds.WithCommandTimeout(1*time.Second)))
	errCtrl.Set(conformance.NewProcessErrorController(env.Processes.APIServer.Get()))

	DeferCleanup(env.Stop)
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow", "temporal"), conformance.TokenStoreTestSuite(store, errCtrl))
var _ = Describe("Cluster Store", Ordered, Label("integration", "slow", "temporal"), conformance.ClusterStoreTestSuite(store, errCtrl))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow", "temporal"), conformance.RBACStoreTestSuite(store, errCtrl))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow", "temporal"), conformance.KeyringStoreTestSuite(store, errCtrl))
