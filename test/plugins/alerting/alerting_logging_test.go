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

var _ = Describe("Alert Logging integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	var alertingClient alertingv1alpha.AlertingClient
	// // test environment references
	var env *test.Environment
	BeforeAll(func() {
		os.Setenv(alerting.LocalBackendEnvToggle, "true")
		os.WriteFile(alerting.LocalAlertManagerPath, []byte(alerting.DefaultAlertManager), 0644)

		// setup managemet server & client
		env = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		// alerting plugin
		alertingClient = alertingv1alpha.NewAlertingClient(env.ManagementClientConn())
	})

	When("The logging API is given invalid input, it should be robust", func() {

	})

	When("The alerting plugin starts...", func() {
		It("Should have empty alert logs", func() {
			existing, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{})
			Expect(err).To(Succeed())
			Expect(existing.Items).To(HaveLen(0))
		})

		It("Should be able to create alert logs", func() {

		})

		It("Should be able to list the most recently available alert logs", func() {

		})
	})
})
