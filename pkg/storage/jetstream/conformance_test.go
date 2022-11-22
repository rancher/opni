package jetstream_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/storage/jetstream"
	"github.com/rancher/opni/pkg/test"
	conformance_storage "github.com/rancher/opni/pkg/test/conformance/storage"
)

func TestJetStream(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "JetStream Storage Suite")
}

var store = new(*jetstream.JetStreamStore)

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
	}
	env.Start(
		test.WithEnableCortex(false),
		test.WithEnableGateway(false),
		test.WithEnableEtcd(false),
		test.WithEnableJetstream(true),
		test.WithEnableDisconnectServer(false),
		test.WithEnableRealtimeServer(false),
	)

	s, err := jetstream.NewJetStreamStore(context.Background(), env.JetStreamConfig())
	Expect(err).NotTo(HaveOccurred())
	*store = s

	DeferCleanup(env.Stop)
})

func init() {
	conformance_storage.BuildTokenStoreTestSuite(store)
	conformance_storage.BuildClusterStoreTestSuite(store)
	conformance_storage.BuildRBACStoreTestSuite(store)
	conformance_storage.BuildKeyringStoreTestSuite(store)
	conformance_storage.BuildKeyValueStoreTestSuite(store)
}
