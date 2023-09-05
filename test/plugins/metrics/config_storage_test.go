package metrics

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/apis"
	"github.com/rancher/opni/pkg/machinery"
	"github.com/rancher/opni/pkg/plugins/apis/system"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"github.com/rancher/opni/pkg/test"
	conformance_driverutil "github.com/rancher/opni/pkg/test/conformance/driverutil"
	"github.com/rancher/opni/pkg/test/testk8s"
	"github.com/rancher/opni/pkg/util/k8sutil"
	"github.com/rancher/opni/plugins/metrics/apis/cortexops"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Config Storage Tests", func() {
	var environment *test.Environment
	BeforeEach(func() {
		environment = &test.Environment{}
	})

	AfterEach(func() {
		DeferCleanup(environment.Stop)
	})

	Context("Defaulting Config Tracker", func() {
		Context("CRD active store", func() {
			var k8sClient client.WithWatch
			BeforeEach(func() {
				Expect(environment.Start(test.WithEnableEtcd(true), test.WithEnableJetstream(false))).To(Succeed())
				restConfig, scheme, err := testk8s.StartK8s(environment.Context(), []string{"../../../config/crd"}, apis.NewScheme())
				Expect(err).NotTo(HaveOccurred())
				k8sClient, err = k8sutil.NewK8sClient(k8sutil.ClientOptions{
					RestConfig: restConfig,
					Scheme:     scheme,
				})
				Expect(err).NotTo(HaveOccurred())
			})
			Context("Etcd default store", func(ctx context.Context) {
				backend, err := machinery.ConfigureStorageBackend(ctx, &environment.GatewayConfig().Spec.Storage)
				Expect(err).NotTo(HaveOccurred())

				server := system.NewKVStoreServer(backend.KeyValueStore("test"))

				client := system.NewKVStoreClient[*cortexops.CapabilityBackendConfigSpec]()

				conformance_driverutil.DefaultingConfigTrackerTestSuite[*cortexops.CapabilityBackendConfigSpec](
					kvutil.WithKey(client, "default-config"),
				)
			})
		})
	})
})
