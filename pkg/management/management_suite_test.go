package management_test

import (
	context "context"
	"crypto/tls"
	"fmt"
	"log"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/storage"
	"github.com/kralicky/opni-monitoring/pkg/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
	"google.golang.org/grpc"
)

func TestManagement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Management Suite")
}

type testVars struct {
	ctrl         *gomock.Controller
	client       management.ManagementClient
	httpEndpoint string
	clusterStore storage.ClusterStore
	tokenStore   storage.TokenStore
	rbacStore    storage.RBACStore
}

func setupManagementServer(vars **testVars) func() {
	return func() {
		tv := &testVars{}
		tv.ctrl = gomock.NewController(GinkgoT())
		ctx, ca := context.WithCancel(context.Background())
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
		server := management.NewServer(conf,
			management.ClusterStore(tv.clusterStore),
			management.TokenStore(tv.tokenStore),
			management.RBACStore(tv.rbacStore),
			management.TLSConfig(&tls.Config{
				Certificates: []tls.Certificate{cert},
			}),
		)
		go func() {
			defer GinkgoRecover()
			if err := server.ListenAndServe(ctx); err != nil {
				log.Println(err)
			}
		}()
		tv.client, err = management.NewClient(ctx,
			management.WithListenAddress(fmt.Sprintf("127.0.0.1:%d", ports[0])),
			management.WithDialOptions(grpc.WithDefaultCallOptions(grpc.WaitForReady(true))),
		)
		Expect(err).NotTo(HaveOccurred())
		tv.httpEndpoint = fmt.Sprintf("http://127.0.0.1:%d", ports[1])
		*vars = tv
		DeferCleanup(ca)
	}
}
