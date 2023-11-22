package metrics_test

import (
	"context"
	"fmt"
	"net"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Impersonated Metrics", Ordered, Label("integration"), func() {
	var environment *test.Environment
	var agent1Cancel context.CancelFunc
	var metricsUrl string
	BeforeAll(func() {
		environment = &test.Environment{}
		Expect(environment.Start()).To(Succeed())
		DeferCleanup(environment.Stop)

		ctx, ca := context.WithCancel(environment.Context())
		agent1Cancel = ca
		err := environment.BootstrapNewAgent("agent1", test.WithContext(ctx), test.WithLocalAgent())
		Expect(err).NotTo(HaveOccurred())
		err = environment.BootstrapNewAgent("agent2")
		Expect(err).NotTo(HaveOccurred())

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
