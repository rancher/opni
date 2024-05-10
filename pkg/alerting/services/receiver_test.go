package services_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher/opni/internal/alertmanager"
	"github.com/rancher/opni/pkg/alerting/services"
	alertingv2 "github.com/rancher/opni/pkg/apis/alerting/v2"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/storage/inmemory"
	"github.com/rancher/opni/pkg/test/testutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
)

var _ = Describe("Receiver service", Label("unit"), Ordered, func() {
	var r services.ReceiverStorageService
	BeforeAll(func() {
		r = services.NewReceiverServer(
			inmemory.NewKeyValueStore[*alertingv2.OpniReceiver](
				func(in *alertingv2.OpniReceiver) *alertingv2.OpniReceiver { return util.ProtoClone(in) },
			),
		)
	})
	When("We use the receiver server", func() {
		It("should put a config", func() {
			inputReceiver := &alertingv2.OpniReceiver{
				Receiver: &alertmanager.Receiver{
					Name: lo.ToPtr("test"),
					SlackConfigs: []*alertmanager.SlackConfig{
						{
							ApiUrl: "https://slack.com/api",
						},
					},
				},
			}

			ref, err := r.PutReceiver(context.TODO(), &alertingv2.OpniReceiver{
				Receiver: inputReceiver.Receiver,
			})
			Expect(err).To(Succeed())
			Expect(ref).ToNot(BeNil())

			recv, err := r.GetReceiver(context.TODO(), ref)
			Expect(err).To(Succeed())
			Expect(recv).ToNot(BeNil())
			Expect(recv.Revision).To(Equal(int64(1)))
			Expect(recv.Receiver).To(testutil.ProtoEqual(inputReceiver.Receiver))

			updateRecv := &alertmanager.Receiver{
				Name: lo.ToPtr("test"),
				SlackConfigs: []*alertmanager.SlackConfig{
					{
						ApiUrl: "https://slack.com/api2",
					},
				},
			}

			_, err = r.PutReceiver(context.TODO(), &alertingv2.OpniReceiver{
				Reference: ref,
				Receiver:  updateRecv,
				Revision:  0,
			})
			Expect(err).To(testutil.MatchStatusCode(storage.ErrConflict))
			ref2, err := r.PutReceiver(context.TODO(), &alertingv2.OpniReceiver{
				Reference: ref,
				Receiver:  updateRecv,
				Revision:  recv.GetRevision(),
			})
			Expect(err).To(Succeed())
			Expect(ref2).ToNot(BeNil())
			Expect(ref2.GetId()).To(Equal(ref.GetId()))

			recv2, err := r.GetReceiver(context.TODO(), ref)
			Expect(err).To(Succeed())
			Expect(recv2).ToNot(BeNil())
			Expect(recv2.Revision).To(Equal(int64(2)))
			Expect(recv2.Receiver).To(testutil.ProtoEqual(updateRecv))
			Expect(recv2.Receiver).NotTo(testutil.ProtoEqual(inputReceiver.Receiver))
		})

		It("should test a receiver", func() {
			apiURL, ok := os.LookupEnv("SLACK_SECRET")
			if !ok {
				Skip("SLACK_SECRET not set")
			}

			_, err := r.TestReceiver(context.TODO(), &alertingv2.OpniReceiver{
				Receiver: &alertmanager.Receiver{
					Name: lo.ToPtr("test"),
					SlackConfigs: []*alertmanager.SlackConfig{
						{
							ApiUrl: apiURL,
						},
					},
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
