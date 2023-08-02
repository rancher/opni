package benchmark_storage

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/test"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util/future"
)

func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Storage Suite")
}

var lmsEtcdF = future.New[[]*etcd.EtcdLockManager]()
var lmsJetstreamOldF = future.New[[]*jetstream.JetstreamLockManager]()

var _ = BeforeSuite(func() {
	testruntime.IfIntegration(func() {
		env := test.Environment{}
		env.Start(
			test.WithEnableGateway(false),
			test.WithEnableEtcd(true),
			test.WithEnableJetstream(false),
		)

		lmsE := make([]*etcd.EtcdLockManager, 7)
		for i := 0; i < 7; i++ {
			l, err := etcd.NewEtcdLockManager(context.Background(), env.EtcdConfig(),
				etcd.WithPrefix("test"),
			)
			Expect(err).NotTo(HaveOccurred())
			lmsE[i] = l
		}

		lmsJOld := make([]*jetstream.JetstreamLockManager, 7)
		for i := 0; i < 7; i++ {
			l, err := jetstream.NewLockManager(env.Context(), env.JetStreamConfig())
			Expect(err).NotTo(HaveOccurred())
			lmsJOld[i] = l
		}

		lmsJetstreamOldF.Set(lmsJOld)

		lmsEtcdF.Set(lmsE)
		// lmsJetstreamF.Set(lmsJ)
		DeferCleanup(env.Stop)
	})
})

var _ = Describe("Etcd lock manager", Ordered, Serial, Label("integration", "slow"), LockManagerBenchmark("etcd", lmsEtcdF))
var _ = Describe("Jetstream (old) lock manager", Ordered, Serial, Label("integration", "slow"), LockManagerBenchmark("jetstream (old)", lmsJetstreamOldF))
