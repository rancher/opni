package management_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rancher/opni/pkg/config/v1beta1"
	"github.com/rancher/opni/pkg/management"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/pkg/util/waitctx"
	"google.golang.org/grpc"
)

func TestManagement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Management Suite")
}

type testVars struct {
	ctrl           *gomock.Controller
	client         management.ManagementClient
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

func setupManagementServer(vars **testVars, opts ...management.ManagementServerOption) func() {
	return func() {
		tv := &testVars{}
		if *vars != nil && (*vars).ctrl != nil {
			tv.ctrl = (*vars).ctrl
		} else {
			tv.ctrl = gomock.NewController(GinkgoT())
		}
		ctx, ca := context.WithCancel(waitctx.Background())
		tv.storageBackend = test.NewTestStorageBackend(ctx, tv.ctrl)
		ports, err := freeport.GetFreePorts(2)
		Expect(err).NotTo(HaveOccurred())
		conf := &v1beta1.ManagementSpec{
			GRPCListenAddress: fmt.Sprintf("tcp://127.0.0.1:%d", ports[0]),
			HTTPListenAddress: fmt.Sprintf("127.0.0.1:%d", ports[1]),
		}
		cert, err := tls.X509KeyPair(test.TestData("localhost.crt"), test.TestData("localhost.key"))
		Expect(err).NotTo(HaveOccurred())
		cds := &testCoreDataSource{
			storageBackend: tv.storageBackend,
			tlsConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}
		server := management.NewServer(ctx, conf, cds, opts...)
		tv.coreDataSource = cds
		tv.ifaces.collector = server
		go func() {
			defer GinkgoRecover()
			if err := server.ListenAndServe(); err != nil {
				test.Log.Error(err)
			}
		}()
		tv.client, err = management.NewClient(ctx,
			management.WithListenAddress(fmt.Sprintf("127.0.0.1:%d", ports[0])),
			management.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
		)
		Expect(err).NotTo(HaveOccurred())
		tv.grpcEndpoint = fmt.Sprintf("127.0.0.1:%d", ports[0])
		tv.httpEndpoint = fmt.Sprintf("http://127.0.0.1:%d", ports[1])
		*vars = tv
		DeferCleanup(func() {
			ca()
			waitctx.Wait(ctx, 5*time.Second)
		})
	}
}
