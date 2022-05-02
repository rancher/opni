package slo_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/cortex/pkg/apis/cortexadmin"
	"github.com/rancher/opni/plugins/slo/pkg/apis/slo"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("SLO RT Module", Ordered, func() {
	var env *test.Environment

	BeforeAll(func() {
		env = &test.Environment{
			TestBin: "../../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		client := env.NewManagementClient()
		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())
		info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())

		p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint})
		env.StartPrometheus(p)
	})
	It("should succeed", func() {
		sloClient := slo.NewSLOClient(env.ManagementClientConn())
		cortexAdminClient := cortexadmin.NewCortexAdminClient(env.ManagementClientConn())

		sloClient.CreateSLO(context.Background(), &slo.ServiceLevelObjective{
			Id: "foo",
			Targets: []*slo.Target{
				{
					ValueX100:  9999,
					TimeWindow: durationpb.New(time.Hour),
				},
			},
		})

		status, err := sloClient.Status(context.Background(), &corev1.Reference{
			Id: "foo",
		})
		Expect(err).To(Succeed())
		Expect(status).NotTo(BeNil())

		Eventually(func() string {
			resp, err := cortexAdminClient.Query(context.Background(), &cortexadmin.QueryRequest{
				Tenants: []string{"agent"},
				Query:   "opni_rt_slo_status",
			})
			if err != nil {
				return ""
			}
			return string(resp.Data)
			// todo: check the validity of the response
		}, 10*time.Second, 100*time.Millisecond).Should(ContainSubstring(`opni_rt_slo_status`))
	})
})
