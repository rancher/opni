package management_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"github.com/rancher/opni-monitoring/pkg/config/v1beta1"
	"github.com/rancher/opni-monitoring/pkg/management"
	"github.com/rancher/opni-monitoring/pkg/storage"
	"github.com/rancher/opni-monitoring/pkg/test"
	"github.com/rancher/opni-monitoring/pkg/waitctx"
	"google.golang.org/grpc"
)

func TestManagement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Management Suite")
}

type testVars struct {
	ctrl         *gomock.Controller
	client       management.ManagementClient
	grpcEndpoint string
	httpEndpoint string
	clusterStore storage.ClusterStore
	tokenStore   storage.TokenStore
	rbacStore    storage.RBACStore
}

func setupManagementServer(vars **testVars, opts ...management.ManagementServerOption) func() {
	return func() {
		tv := &testVars{}
		if *vars != nil && (*vars).ctrl != nil {
			tv.ctrl = (*vars).ctrl
		} else {
			tv.ctrl = gomock.NewController(GinkgoT())
		}
		ctx, ca := context.WithCancel(waitctx.FromContext(context.Background()))
		tv.clusterStore = test.NewTestClusterStore(tv.ctrl)
		tv.tokenStore = test.NewTestTokenStore(ctx, tv.ctrl)
		tv.rbacStore = test.NewTestRBACStore(tv.ctrl)
		ports, err := freeport.GetFreePorts(2)
		Expect(err).NotTo(HaveOccurred())
		conf := &v1beta1.ManagementSpec{
			GRPCListenAddress: fmt.Sprintf("tcp://127.0.0.1:%d", ports[0]),
			HTTPListenAddress: fmt.Sprintf("127.0.0.1:%d", ports[1]),
		}
		cert, err := tls.X509KeyPair(test.TestData("localhost.crt"), test.TestData("localhost.key"))
		Expect(err).NotTo(HaveOccurred())
		server := management.NewServer(ctx, conf,
			append([]management.ManagementServerOption{
				management.ClusterStore(tv.clusterStore),
				management.TokenStore(tv.tokenStore),
				management.RBACStore(tv.rbacStore),
				management.TLSConfig(&tls.Config{
					Certificates: []tls.Certificate{cert},
				}),
			}, opts...)...)
		go func() {
			defer GinkgoRecover()
			if err := server.ListenAndServe(); err != nil {
				log.Println(err)
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
