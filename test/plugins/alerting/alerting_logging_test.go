package alerting_test

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/timestamppb"

	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/plugins/alerting/pkg/alerting"

	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Alert Logging integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	testIds := map[string]string{
		"test1": uuid.New().String(),
	}
	beforeTime := time.Now()
	// purge data from other tests
	BeforeEach(func() {
		alerting.AlertPath = "alerttestdata/logs"
		err := os.RemoveAll(alerting.AlertPath)
		Expect(err).To(BeNil())
		err = os.MkdirAll(alerting.AlertPath, 0755)
		Expect(err).To(BeNil())
	})

	When("The logging API is given invalid input, it should be robust", func() {
		//TODO
	})

	When("The alerting plugin starts...", func() {
		It("Should have empty alert logs", func() {
			items, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
				Labels: []string{},
			})
			Expect(err).To(BeNil())
			Expect(items.Items).To(HaveLen(0))
		})

		It("Should be able to create alert logs", func() {
			alertLog := &corev1.AlertLog{
				ConditionId: &corev1.Reference{Id: testIds["test1"]},

				Timestamp: &timestamppb.Timestamp{
					Seconds: time.Now().Unix(),
				},
			}
			_, err := alertingClient.CreateAlertLog(ctx, alertLog)
			Expect(err).To(BeNil())

			items, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{})
			Expect(err).To(BeNil())
			Expect(items.Items).To(HaveLen(1))
		})

		It("Should be able to list the most recently available alert logs", func() {
			items, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
				Labels: []string{},
				StartTimestamp: &timestamppb.Timestamp{
					Seconds: beforeTime.Unix(),
				},
			})
			Expect(err).To(BeNil())
			Expect(items.Items).To(HaveLen(1))
		})

		It("Should be able to list previously available alert logs", func() {

		})
	})
})
