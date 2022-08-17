package alerting_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/pkg/alerting/condition"
	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	managementv1 "github.com/rancher/opni/pkg/apis/management/v1"
	"github.com/rancher/opni/pkg/util/waitctx"

	"github.com/rancher/opni/pkg/test"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

var _ = Describe("Alerting Conditions integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()

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
			//TODO

		})

		Specify("Update Alert Condition API should be robust to invalid input", func() {
			//TODO
		})

		Specify("List Alert Condition API should be robust to invalid input", func() {
			//TODO
		})

		Specify("Delete Alert Condition API should be robust to invalid input", func() {
			//TODO
		})
	})

	When("The alerting plugin starts...", func() {
		It("Should be create [system] type alert conditions, when that code loads", func() {
			conditions, err := alertingClient.ListAlertConditions(ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(Succeed())
			Expect(conditions.Items).To(HaveLen(0))

			client := env.NewManagementClient()
			token, err := client.CreateBootstrapToken(context.Background(), &managementv1.CreateBootstrapTokenRequest{
				Ttl: durationpb.New(time.Hour),
			})
			Expect(err).NotTo(HaveOccurred())
			info, err := client.CertsInfo(context.Background(), &emptypb.Empty{})
			Expect(err).To(Succeed())

			// on agent startup expect an alert condition to be created
			ctxca, ca := context.WithCancel(waitctx.FromContext(context.Background()))
			p, _ := env.StartAgent("agent", token, []string{info.Chain[len(info.Chain)-1].Fingerprint}, test.WithContext(ctxca))
			Expect(p).NotTo(Equal(0))
			time.Sleep(time.Second)
			newConds, err := alertingClient.ListAlertConditions(ctx, &alertingv1alpha.ListAlertConditionRequest{})
			Expect(err).To(Succeed())
			Expect(newConds.Items).To(HaveLen(1))

			// kill the agent
			ca()
			time.Sleep(time.Millisecond * 100)
			logs, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{})
			Expect(err).To(Succeed())
			Expect(logs.Items).NotTo(HaveLen(0))
			filteredLogs, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
				Labels: condition.OpniDisconnect.Labels,
			})
			Expect(err).To(Succeed())
			Expect(filteredLogs.Items).ToNot(HaveLen(0))
		})
	})
})
