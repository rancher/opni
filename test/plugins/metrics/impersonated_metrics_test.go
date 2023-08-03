package metrics_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/test"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Impersonated Metrics", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var agent1Cancel context.CancelFunc
	var metricsUrl string
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		DeferCleanup(environment.Stop)

		client := environment.NewManagementClient()

		certsInfo, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
		Expect(err).NotTo(HaveOccurred())
		fingerprint := certsInfo.Chain[len(certsInfo.Chain)-1].Fingerprint
		Expect(fingerprint).NotTo(BeEmpty())

		token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
			Ttl: durationpb.New(1 * time.Hour),
		})
		Expect(err).NotTo(HaveOccurred())

		ctx, ca := context.WithCancel(environment.Context())
		agent1Cancel = ca
		_, errC := environment.StartAgent("agent1", token, []string{fingerprint}, test.WithContext(ctx), test.WithLocalAgent())
		Eventually(errC).Should(Receive(BeNil()))

		_, errC = environment.StartAgent("agent2", token, []string{fingerprint})
		Eventually(errC).Should(Receive(BeNil()))

		hostport := environment.GatewayConfig().Spec.MetricsListenAddress
		_, port, _ := net.SplitHostPort(hostport)
		metricsUrl = fmt.Sprintf("http://127.0.0.1:%s/metrics", port)
	})

	When("scraping metrics from the gateway", func() {
		It("should contain the impersonated metrics from the local agent", func() {

			Eventually(func() error {
				return promtestutil.ScrapeAndCompare(metricsUrl, strings.NewReader(`
# HELP opni_agent_status_summary Agent status summary
# TYPE opni_agent_status_summary gauge
opni_agent_status_summary{summary="Healthy",zz_opni_impersonate_as="agent1"} 1
opni_agent_status_summary{summary="Healthy",zz_opni_impersonate_as="agent2"} 1
`[1:]), "opni_agent_status_summary")
			}).Should(Succeed())
		})
	})

	When("the agent is stopped", func() {
		It("should update the impersonated metrics", func() {
			agent1Cancel()

			Eventually(func() error {
				return promtestutil.ScrapeAndCompare(metricsUrl, strings.NewReader(`
# HELP opni_agent_status_summary Agent status summary
# TYPE opni_agent_status_summary gauge
opni_agent_status_summary{summary="Disconnected",zz_opni_impersonate_as="agent1"} 1
opni_agent_status_summary{summary="Healthy",zz_opni_impersonate_as="agent2"} 1
`[1:]), "opni_agent_status_summary")
			}).Should(Succeed())
		})
	})

	When("the agent is restarted", func() {
		It("should update the impersonated metrics", func() {
			_, errC := environment.StartAgent("agent1", nil, nil, test.WithLocalAgent())
			Eventually(errC).Should(Receive(BeNil()))
			Eventually(func() error {
				return promtestutil.ScrapeAndCompare(metricsUrl, strings.NewReader(`
# HELP opni_agent_status_summary Agent status summary
# TYPE opni_agent_status_summary gauge
opni_agent_status_summary{summary="Healthy",zz_opni_impersonate_as="agent1"} 1
opni_agent_status_summary{summary="Healthy",zz_opni_impersonate_as="agent2"} 1
`[1:]), "opni_agent_status_summary")
			}).Should(Succeed())
		})
	})
})
