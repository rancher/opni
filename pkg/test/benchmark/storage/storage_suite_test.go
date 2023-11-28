package benchmark_storage

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/logger"
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
var lmsJetstreamF = future.New[[]*jetstream.LockManager]()

var _ = BeforeSuite(func() {
	testruntime.IfIntegration(func() {
		env := test.Environment{}
		env.Start(
			test.WithEnableGateway(false),
			test.WithEnableEtcd(true),
			test.WithEnableJetstream(true),
		)

		lmsE := make([]*etcd.EtcdLockManager, 7)
		for i := 0; i < 7; i++ {
			cli, err := etcd.NewEtcdClient(context.Background(), env.EtcdConfig())
			Expect(err).To(Succeed())

			l := etcd.NewEtcdLockManager(cli, "test", logger.NewNop())
			Expect(err).NotTo(HaveOccurred())
			lmsE[i] = l
		}
		lmsJ := make([]*jetstream.LockManager, 7)
		for i := 0; i < 7; i++ {
			_, js, err := jetstream.AcquireJetstreamConn(context.Background(), env.JetStreamConfig(), logger.New().WithGroup("js"))
			Expect(err).To(Succeed())

			j := jetstream.NewLockManager(context.Background(), js, "test", logger.NewNop())
			lmsJ[i] = j
		}

		lmsEtcdF.Set(lmsE)
		lmsJetstreamF.Set(lmsJ)
		DeferCleanup(env.Stop, "Test Suite Finished")
	})
})

// Manually enable benchmarks by commenting out

// var _ = Describe("Etcd lock manager", Ordered, Serial, Label("integration", "slow"), LockManagerBenchmark("etcd", lmsEtcdF))
// var _ = Describe("Jetstream lock manager", Ordered, Serial, Label("integration", "slow"), LockManagerBenchmark("jetstream", lmsJetstreamF))
