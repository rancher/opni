package crds_test

import (
	"testing"

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

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
		CRDDirectoryPaths: []string{
			"../../../config/crd/bases",
		},
	}
	config, _, err := env.StartK8s()
	Expect(err).NotTo(HaveOccurred())

	store.Set(crds.NewCRDStore(crds.WithRestConfig(config)))

	DeferCleanup(env.Stop)
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow"), conformance.TokenStoreTestSuite(store))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow"), conformance.RBACStoreTestSuite(store))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow"), conformance.KeyringStoreTestSuite(store))
