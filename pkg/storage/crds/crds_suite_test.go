package crds_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	opnicorev1beta1 "github.com/rancher/opni/apis/core/v1beta1"
	monitoringv1beta1 "github.com/rancher/opni/apis/monitoring/v1beta1"
	"github.com/rancher/opni/pkg/storage/crds"
	. "github.com/rancher/opni/pkg/test/conformance/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/rancher/opni/pkg/util/waitctx"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRDs Storage Suite")
}

var store = future.New[*crds.CRDStore]()

var _ = BeforeSuite(func() {
	testruntime.IfLabelFilterMatches(Label("integration", "slow"), func() {
		ctx, ca := context.WithCancel(waitctx.Background())
		s := scheme.Scheme
		opnicorev1beta1.AddToScheme(s)
		monitoringv1beta1.AddToScheme(s)
		config, _, err := testk8s.StartK8s(ctx, []string{"../../../config/crd/bases"}, s)
		Expect(err).NotTo(HaveOccurred())

		store.Set(crds.NewCRDStore(crds.WithRestConfig(config)))

		DeferCleanup(func() {
			ca()
			waitctx.Wait(ctx)
		})
	})
})

var _ = Describe("Token Store", Ordered, Label("integration", "slow"), TokenStoreTestSuite(store))
var _ = Describe("RBAC Store", Ordered, Label("integration", "slow"), RBACStoreTestSuite(store))
var _ = Describe("Keyring Store", Ordered, Label("integration", "slow"), KeyringStoreTestSuite(store))
