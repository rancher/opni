package management_test

import (
	"context"
	"fmt"
	"log"

	"github.com/golang/mock/gomock"
	"github.com/kralicky/opni-monitoring/pkg/config/v1beta1"
	"github.com/kralicky/opni-monitoring/pkg/management"
	"github.com/kralicky/opni-monitoring/pkg/test"
	"github.com/phayes/freeport"
	"google.golang.org/protobuf/types/known/emptypb"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Server", Ordered, func() {
	var ctrl *gomock.Controller
	var client management.ManagementClient
	BeforeAll(func() {
		ctrl = gomock.NewController(GinkgoT())
		ports, err := freeport.GetFreePorts(2)
		Expect(err).ToNot(HaveOccurred())
		conf := &v1beta1.ManagementSpec{
			GRPCListenAddress: fmt.Sprintf("tcp://127.0.0.1:%d", ports[0]),
			HTTPListenAddress: fmt.Sprintf("127.0.0.1:%d", ports[1]),
		}
		ctx, ca := context.WithCancel(context.Background())
		server := management.NewServer(conf,
			management.ClusterStore(test.NewTestClusterStore(ctrl)),
			management.RBACStore(test.NewTestRBACStore(ctrl)),
			management.TokenStore(test.NewTestTokenStore(ctx, ctrl)),
			management.TLSConfig(nil),
		)
		go func() {
			defer GinkgoRecover()
			if err := server.ListenAndServe(ctx); err != nil {
				log.Println(err)
			}
		}()
		client, err = management.NewClient(ctx, management.WithListenAddress(fmt.Sprintf("127.0.0.1:%d", ports[0])))
		Expect(err).ToNot(HaveOccurred())

		DeferCleanup(ca)
	})

	When("the management server starts", func() {
		It("should not have any stored items", func() {
			clusters, err := client.ListClusters(context.Background(), &management.ListClustersRequest{})
			Expect(err).ToNot(HaveOccurred())
			Expect(clusters.Items).To(BeEmpty())

			tokens, err := client.ListBootstrapTokens(context.Background(), &emptypb.Empty{})
			Expect(err).ToNot(HaveOccurred())
			Expect(tokens.Items).To(BeEmpty())

			roles, err := client.ListRoles(context.Background(), &emptypb.Empty{})
			Expect(err).ToNot(HaveOccurred())
			Expect(roles.Items).To(BeEmpty())

			roleBindings, err := client.ListRoleBindings(context.Background(), &emptypb.Empty{})
			Expect(err).ToNot(HaveOccurred())
			Expect(roleBindings.Items).To(BeEmpty())
		})
	})
})
