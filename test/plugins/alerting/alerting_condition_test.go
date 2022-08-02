package alerting_test

import (
	"context"
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

var _ = Describe("Alerting Conditions integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	var alertingClient alertingv1alpha.AlertingClient
	// // test environment references
	var env *test.Environment
	BeforeAll(func() {
		var err error
		Expect(err).To(BeNil())
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

	When("The alerting condition API is passed invalid input it should be robust", func() {
		Specify("Create Alert Condition API should be robust to invalid input", func() {
			toTestCreateCondition := []InvalidInputs{
				{
					req: &alertingv1alpha.AlertCondition{},
					err: fmt.Errorf("invalid input"),
				},
			}

			for _, invalidInput := range toTestCreateCondition {
				_, err := alertingClient.CreateAlertCondition(ctx, invalidInput.req.(*alertingv1alpha.AlertCondition))
				Expect(err).To(HaveOccurred())
			}

		})

		Specify("Get Alert Condition API should be robust to invalid input", func() {
			// toTestCreateCondition := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertCondition{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }

			// for _, invalidInput := range toTestCreateCondition {
			// 	_, err := alertingClient.GetAlertCondition(ctx, invalidInput.req.(*alertingv1alpha.AlertCondition))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }

		})

		Specify("Update Alert Condition API should be robust to invalid input", func() {
			// toTestCreateCondition := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertCondition{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }

			// for _
		})

		Specify("List Alert Condition API should be robust to invalid input", func() {
			// toTestCreateCondition := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertCondition{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }

			// for _
		})

		Specify("Delete Alert Condition API should be robust to invalid input", func() {
		})
	})
})
