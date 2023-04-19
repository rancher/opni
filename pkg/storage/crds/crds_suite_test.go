package crds_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/rancher/opni/pkg/test/testruntime"

	conformance "github.com/rancher/opni/pkg/storage/conformance"
	"github.com/rancher/opni/pkg/storage/crds"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/pkg/util/waitctx"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRDs Storage Suite")
}

var store = future.New[*crds.CRDStore]()

var _ = BeforeSuite(func() {
	testruntime.IfLabelFilterMatches(Label("integration", "slow"), func() {
		ctx, ca := context.WithCancel(waitctx.Background())
		config, _, err := testk8s.StartK8s(ctx, []string{"../../../config/crd/bases"})
		Expect(err).NotTo(HaveOccurred())

		store.Set(crds.NewCRDStore(crds.WithRestConfig(config)))

		DeferCleanup(func() {
			ca()
			waitctx.Wait(ctx)
		})
	})
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow"), conformance.TokenStoreTestSuite(store))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow"), conformance.RBACStoreTestSuite(store))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow"), conformance.KeyringStoreTestSuite(store))
