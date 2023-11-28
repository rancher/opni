package etcd_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/etcd"
	"github.com/rancher/opni/pkg/test"
	. "github.com/rancher/opni/pkg/test/conformance/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testruntime"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/future"
	"github.com/samber/lo"
	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestEtcd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcd Storage Suite")
}

func clientFromConfig(
	ctx context.Context,
	conf *v1beta1.EtcdStorageSpec,
) (*clientv3.Client, *clientv3.Config, error) {
	var tlsConfig *tls.Config
	if conf.Certs != nil {
		var err error
		tlsConfig, err = util.LoadClientMTLSConfig(*conf.Certs)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to load client TLS config: %w", err)
		}
	}

	clientConfig := &clientv3.Config{
		Endpoints: conf.Endpoints,
		TLS:       tlsConfig,
		Context:   context.WithoutCancel(ctx),
	}
	cli, err := clientv3.New(*clientConfig)
	if err != nil {
		return nil, clientConfig, fmt.Errorf("failed to create etcd client: %w", err)
	}
	return cli, clientConfig, nil
}

var store = future.New[*etcd.EtcdStore]()

var lmF = future.New[storage.LockManager]()
var lmSet = future.New[lo.Tuple3[
	storage.LockManager, storage.LockManager, storage.LockManager,
]]()
var snapshotter = future.New[*etcd.Snapshotter]()

var _ = BeforeSuite(func() {
	testruntime.IfIntegration(func() {
		env := test.Environment{}
		env.Start(
			test.WithEnableGateway(false),
			test.WithEnableEtcd(true),
			test.WithEnableJetstream(false),
		)

		client, err := etcd.NewEtcdStore(context.Background(), env.EtcdConfig(),
			etcd.WithPrefix("test"),
		)
		Expect(err).To(Succeed())

		cli, err := etcd.NewEtcdClient(context.Background(), env.EtcdConfig())
		Expect(err).To(Succeed())

		lm := etcd.NewEtcdLockManager(cli, "test", logger.NewNop())
		lmF.Set(lm)

		x, err := etcd.NewEtcdClient(context.Background(), env.EtcdConfig())
		Expect(err).To(Succeed())
		lmX := etcd.NewEtcdLockManager(x, "test", logger.NewNop())

		y, err := etcd.NewEtcdClient(context.Background(), env.EtcdConfig())
		Expect(err).To(Succeed())
		lmY := etcd.NewEtcdLockManager(y, "test", logger.NewNop())
		Expect(err).To(Succeed())

		z, err := etcd.NewEtcdClient(context.Background(), env.EtcdConfig())
		Expect(err).To(Succeed())
		lmZ := etcd.NewEtcdLockManager(z, "test", logger.NewNop())

		lmSet.Set(lo.Tuple3[storage.LockManager, storage.LockManager, storage.LockManager]{
			A: lmX, B: lmY, C: lmZ,
		})

		Expect(err).NotTo(HaveOccurred())
		store.Set(client)

		cli, config, err := clientFromConfig(context.Background(), env.EtcdConfig())
		Expect(err).To(Succeed())
		dataDir := filepath.Join(env.GetTempDirectory(), "etcd")

		s := etcd.NewSnapshotter(config, cli, dataDir, logger.New())

		snapshotter.Set(s)
		DeferCleanup(env.Stop, "Test Suite Finished")
	})
})

var _ = Describe("Etcd Token Store", Ordered, Label("integration", "slow"), TokenStoreTestSuite(store))
var _ = Describe("Etcd Cluster Store", Ordered, Label("integration", "slow"), ClusterStoreTestSuite(store))
var _ = Describe("Etcd RBAC Store", Ordered, Label("integration", "slow"), RBACStoreTestSuite(store))
var _ = Describe("Etcd Keyring Store", Ordered, Label("integration", "slow"), KeyringStoreTestSuite(store))
var _ = Describe("Etcd KV Store", Ordered, Label("integration", "slow"), KeyValueStoreTestSuite(store, NewBytes, Equal))
var _ = Describe("Etcd Lock Manager", Ordered, Label("integration", "slow"), LockManagerTestSuite(lmF, lmSet))
var _ = Describe("Etcd Backup Restore", Ordered, Label("integration", "slow"), SnapshotSuiteTest(snapshotter))
