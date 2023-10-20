package management_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/plugins"
	"github.com/rancher/opni/pkg/plugins/hooks"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test/freeport"
	mock_storage "github.com/rancher/opni/pkg/test/mock/storage"
	_ "github.com/rancher/opni/pkg/test/setup"
	"github.com/rancher/opni/pkg/test/testdata"
	"github.com/rancher/opni/pkg/test/testlog"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
)

func TestManagement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Management Suite")
}

type testVars struct {
	ctrl           *gomock.Controller
	client         managementv1.ManagementClient
	grpcEndpoint   string
	httpEndpoint   string
	coreDataSource management.CoreDataSource
	storageBackend storage.Backend
	ifaces         struct {
		collector prometheus.Collector
	}
}

type testCoreDataSource struct {
	storageBackend storage.Backend
	tlsConfig      *tls.Config
}

func (t *testCoreDataSource) StorageBackend() storage.Backend {
	return t.storageBackend
}

func (t *testCoreDataSource) TLSConfig() *tls.Config {
	return t.tlsConfig
}

func setupManagementServer(vars **testVars, pl plugins.LoaderInterface, opts ...management.ManagementServerOption) func() {
	return func() {
		tv := &testVars{}
		if *vars != nil && (*vars).ctrl != nil {
			tv.ctrl = (*vars).ctrl
		} else {
			tv.ctrl = gomock.NewController(GinkgoT())
		}
		ctx, ca := context.WithCancel(context.Background())
		tv.storageBackend = mock_storage.NewTestStorageBackend(ctx, tv.ctrl)
		ports := freeport.GetFreePorts(2)
		conf := &v1beta1.ManagementSpec{
			GRPCListenAddress: fmt.Sprintf("tcp://127.0.0.1:%d", ports[0]),
			HTTPListenAddress: fmt.Sprintf("127.0.0.1:%d", ports[1]),
		}
		cert, err := tls.X509KeyPair(testdata.TestData("localhost.crt"), testdata.TestData("localhost.key"))
		Expect(err).NotTo(HaveOccurred())
		cds := &testCoreDataSource{
			storageBackend: tv.storageBackend,
			tlsConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}
		server := management.NewServer(ctx, conf, cds, pl, opts...)
		tv.coreDataSource = cds
		tv.ifaces.collector = server
		pl.Hook(hooks.OnLoadingCompleted(func(int) {
			defer GinkgoRecover()
			if err := server.ListenAndServe(ctx); err != nil {
				testlog.Log.Error("error", logger.Err(err))
			}
		}))
		tv.client, err = management.NewClient(ctx,
			management.WithListenAddress(fmt.Sprintf("127.0.0.1:%d", ports[0])),
			management.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
		)
		Expect(err).NotTo(HaveOccurred())
		tv.grpcEndpoint = fmt.Sprintf("127.0.0.1:%d", ports[0])
		tv.httpEndpoint = fmt.Sprintf("http://127.0.0.1:%d", ports[1])
		*vars = tv
		DeferCleanup(ca)
	}
}
