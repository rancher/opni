package alerting_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

var _ = Describe("Alerting", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	var alertingClient alertingv1alpha.AlertingClient
	// test environment references
	var env *test.Environment
	BeforeAll(func() {
		// use alertmanager binary
		os.Setenv(alerting.LocalBackendEnvToggle, "true")

		// setup managemet server & client
		env = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		// alerting plugin
		alertingClient = alertingv1alpha.NewAlertingClient(env.ManagementClientConn())
	})

	When("The alerting plugin starts", func() {
		Specify("The runtime backend should connect", func() {
			var err error
			Expect(err).To(BeNil())
		})

		It("Should have loaded preconfigured alert conditions", func() {
			_, err := alertingClient.ListAlertConditions(
				ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(HaveOccurred())
		})
	})
})
