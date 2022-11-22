//go:build !noetcd

package etcd_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/test"
	conformance_storage "github.com/rancher/opni/pkg/test/conformance/storage"
)

func TestEtcd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcd Storage Suite")
}

var store = new(*etcd.EtcdStore)

var _ = BeforeSuite(func() {
	env := test.Environment{
		TestBin: "../../../testbin/bin",
	}
	env.Start(
		test.WithEnableCortex(false),
		test.WithEnableGateway(false),
		test.WithEnableEtcd(true),
		test.WithEnableJetstream(false),
		test.WithEnableDisconnectServer(false),
		test.WithEnableRealtimeServer(false),
	)

	*store = etcd.NewEtcdStore(context.Background(), env.EtcdConfig(),
		etcd.WithPrefix("test"),
	)

	DeferCleanup(env.Stop)
})

func init() {
	conformance_storage.BuildTokenStoreTestSuite(store)
	conformance_storage.BuildClusterStoreTestSuite(store)
	conformance_storage.BuildRBACStoreTestSuite(store)
	conformance_storage.BuildKeyringStoreTestSuite(store)
	conformance_storage.BuildKeyValueStoreTestSuite(store)
}
