package logs_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/timestamppb"

	alertingv1alpha "github.com/rancher/opni/pkg/apis/alerting/v1alpha"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/test"
)

var _ = Describe("Alert Logging integration tests", Ordered, Label(test.Unit, test.Slow), func() {
	ctx := context.Background()
	testIds := map[string]string{
		"test1": uuid.New().String(),
	}
	beforeTime := time.Now()
	//var existingLogCount int
	// purge data from other tests
	BeforeEach(func() {
		//FIXME
		//alerting.AlertPath = "../../../dev/alerttestdata/logs"
		//err := os.RemoveAll(alerting.AlertPath)
		//Expect(err).To(BeNil())
		//err = os.MkdirAll(alerting.AlertPath, 0777)
		//Expect(err).To(BeNil())
	})

	When("The logging API is given invalid input, it should be robust", func() {
		//TODO
	})

	When("The alerting plugin starts...", func() {
		It("Should be able to list the available logs", func() {
			/*items*/ _, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
				Labels: []string{},
			})
			Expect(err).To(BeNil())
			//existingLogCount = len(items.Items)
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

			/*items*/
			_, err = alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{})
			Expect(err).To(BeNil())
			//FIXME: something broken here
			//Expect(len(items.Items)).To(BeNumerically(">=", existingLogCount+1))
		})

		It("Should be able to list the most recently available alert logs", func() {
			/*items*/ _, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
				Labels: []string{},
				StartTimestamp: &timestamppb.Timestamp{
					Seconds: beforeTime.Unix(),
				},
			})
			Expect(err).To(BeNil())
			//FIXME: something broken here
			//Expect(items.Items).To(HaveLen(existingLogCount + 1))
		})

		It("Should be able to list previously available alert logs", func() {
			t := time.Now().Unix()
			/*items*/ _, err := alertingClient.ListAlertLogs(ctx, &alertingv1alpha.ListAlertLogRequest{
				Labels: []string{},
				EndTimestamp: &timestamppb.Timestamp{
					Seconds: t,
				},
			})
			Expect(err).To(BeNil())
			// checks for timestamp equality, because range rounds up to the nearest second
			//FIXME: something broken here
			//Expect(len(items.Items)).To(BeNumerically(">=", existingLogCount+1)) // need >= because async flaky
		})
	})
})
