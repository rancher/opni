package alerting_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"
)

var _ = Describe("Alerting Endpoints integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	// ctx := context.Background()
	// var alertingClient alertingv1alpha.AlertingClient
	// test environment references
	var env *test.Environment
	BeforeAll(func() {
		// use alertmanager binary
		os.Setenv(alerting.LocalBackendEnvToggle, "true")
		os.WriteFile(alerting.LocalAlertManagerPath, []byte(alerting.DefaultAlertManager), 0644)

		// setup managemet server & client
		env = &test.Environment{
			TestBin: "../../../testbin/bin",
		}
		Expect(env.Start()).To(Succeed())
		DeferCleanup(env.Stop)

		// alerting plugin
		//alertingClient = alertingv1alpha.NewAlertingClient(env.ManagementClientConn())
	})

	When("The API is passed invalid input, handle it", func() {
		Specify("Create Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }

			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.CreateAlertEndpoint(ctx, invalidInput.req.(*alertingv1alpha.AlertEndpoint))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }

		})

		Specify("Get Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }

			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.GetAlertEndpoint(ctx, invalidInput.req.(*corev1.Reference))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }

		})

		Specify("Update Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.UpdateAlertEndpoint(ctx, invalidInput.req.(*alertingv1alpha.UpdateAlertEndpointRequest))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("List Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.ListAlertEndpoints(ctx, invalidInput.req.(*alertingv1alpha.ListAlertEndpointsRequest))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("Delete Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: fmt.Errorf("invalid input"),
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.DeleteAlertEndpoint(ctx, invalidInput.req.(*corev1.Reference))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("Test Alert Endpoint API should be robust to invalid input", func() {
			// toTestCreateEndpoint := []InvalidInputs{
			// 	{
			// 		req: &alertingv1alpha.AlertEndpoint{},
			// 		err: shared.AlertingErrNotImplemented,
			// 	},
			// }
			// for _, invalidInput := range toTestCreateEndpoint {
			// 	_, err := alertingClient.TestAlertEndpoint(ctx, invalidInput.req.(*alertingv1alpha.TestAlertEndpointRequest))
			// 	Expect(err).To(HaveOccurred())
			// 	Expect(err.Error()).To(Equal(invalidInput.err.Error()))
			// }
		})

		Specify("Get Implementation From Endpoint API should be robust to invalid input", func() {

		})

		Specify("Create Endpoint Implementation API should be robust to invalid input ", func() {

		})

		Specify("Update Endpoint Implementation API should be robust to invalid input ", func() {

		})

		Specify("Delete Endpoint Implementation API should be robust to invalid input", func() {

		})

	})

	When("The alerting plugin starts", func() {
		It("Should be able to CRUD (Reusable K,V groups) Alert Endpoints", func() {

		})
	})
})
