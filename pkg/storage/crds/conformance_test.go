package crds_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/storage/crds"
	"github.com/rancher/opni/pkg/test"
	conformance_storage "github.com/rancher/opni/pkg/test/conformance/storage"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRDs Storage Suite")
}

var store = new(*crds.CRDStore)

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
		CRDDirectoryPaths: []string{
			"../../../config/crd/bases",
		},
	}
	config, _, err := env.StartK8s()
	Expect(err).NotTo(HaveOccurred())

	*store = crds.NewCRDStore(crds.WithRestConfig(config))

	DeferCleanup(env.Stop)
})

func init() {
	conformance_storage.BuildTokenStoreTestSuite(store)
	conformance_storage.BuildRBACStoreTestSuite(store)
	conformance_storage.BuildKeyringStoreTestSuite(store)
}
