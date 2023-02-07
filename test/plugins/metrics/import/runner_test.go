package _import

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/test"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func newRespondingReader() *mockRemoteReader {
	return &mockRemoteReader{
		Responses: []*prompb.ReadResponse{
			{
				Results: []*prompb.QueryResult{
					{
						Timeseries: []*prompb.TimeSeries{
							{
								Labels: []prompb.Label{},
								Samples: []prompb.Sample{
									{
										Value:     100,
										Timestamp: 100,
									},
								},
								Exemplars: []prompb.Exemplar{
									{
										Labels:    nil,
										Value:     0,
										Timestamp: 0,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

var _ = Describe("Target Runner", Ordered, Label(test.Unit), func() {
	var (
		failingReader = &mockRemoteReader{
			Error: fmt.Errorf("failed"),
		}

		runner agent.TargetRunner

		writerClient *mockRemoteWriteClient

		target = &remoteread.Target{
			Meta: &remoteread.TargetMeta{
				Name:      "test",
				ClusterId: "00000-00000",
			},
			Spec: &remoteread.TargetSpec{
				Endpoint: "http://127.0.0.1:9090/api/v1/read",
			},
			Status: nil,
		}
	)

	BeforeEach(func() {
		lg := logger.NewPluginLogger().Named("test-runner")

		writerClient = &mockRemoteWriteClient{}

		runner = agent.NewTargetRunner(lg)
		runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
			return writerClient
		}))
	})

	When("target runner cannot reach target endpoint", func() {
		It("should fail", func() {
			runner.SetRemoteReaderClient(failingReader)

			err := runner.Start(
				target,
				&remoteread.Query{
					StartTimestamp: &timestamppb.Timestamp{},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
					},
					Matchers: nil,
				})
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() corev1.TaskState {
				status, err = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(corev1.TaskState_Failed))

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp:    &timestamppb.Timestamp{},
					LastReadTimestamp: nil,
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "failed to read from target endpoint: failed",
				State:   corev1.TaskState_Failed,
			}

			AssertTargetStatus(expected, status)
		})
	})

	When("editing and restarting failed import", func() {
		It("should should", func() {
			runner.SetRemoteReaderClient(newRespondingReader())

			err := runner.Start(
				target,
				&remoteread.Query{
					StartTimestamp: &timestamppb.Timestamp{},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
					},
					Matchers: nil,
				})
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() corev1.TaskState {
				status, err = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(corev1.TaskState_Completed))

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp: &timestamppb.Timestamp{},
					LastReadTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "",
				State:   corev1.TaskState_Completed,
			}

			AssertTargetStatus(expected, status)
			Expect(len(writerClient.Payloads)).To(Equal(1))
		})
	})

	When("target runner can reach target endpoint", func() {
		It("should complete", func() {
			runner.SetRemoteReaderClient(newRespondingReader())

			err := runner.Start(
				target,
				&remoteread.Query{
					StartTimestamp: &timestamppb.Timestamp{},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
					},
					Matchers: nil,
				})
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() corev1.TaskState {
				status, err = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(corev1.TaskState_Completed))

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp: &timestamppb.Timestamp{},
					LastReadTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "",
				State:   corev1.TaskState_Completed,
			}

			AssertTargetStatus(expected, status)
			Expect(len(writerClient.Payloads)).To(Equal(1))
		})
	})
})
