package metrics_test

import (
	"fmt"
	"math"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"github.com/rancher/opni/plugins/metrics/pkg/agent"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
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

var _ = Describe("Target Runner", Ordered, Label("unit"), func() {
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

		query = &remoteread.Query{
			StartTimestamp: &timestamppb.Timestamp{},
			EndTimestamp: &timestamppb.Timestamp{
				Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
			},
			Matchers: nil,
		}
	)

	BeforeEach(func() {
		lg := logger.NewPluginLogger().WithGroup("test-runner")

		writerClient = &mockRemoteWriteClient{}

		runner = agent.NewTargetRunner(lg)
		runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
			return writerClient
		}))
	})

	When("target status is not running", func() {
		It("cannot get status", func() {
			status, err := runner.GetStatus("test")
			AssertTargetStatus(&remoteread.TargetStatus{
				Progress: nil,
				Message:  "",
				State:    remoteread.TargetState_NotRunning,
			}, status)
			Expect(err).ToNot(HaveOccurred())
		})

		It("cannot stop", func() {
			err := runner.Stop("test")
			Expect(err).To(HaveOccurred())
		})
	})

	When("target runner cannot reach target endpoint", func() {
		It("should fail", func() {
			runner.SetRemoteReaderClient(failingReader)

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Failed))

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp:    &timestamppb.Timestamp{},
					LastReadTimestamp: &timestamppb.Timestamp{},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: agent.TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "failed to read from target endpoint: failed",
				State:   remoteread.TargetState_Failed,
			}

			AssertTargetStatus(expected, status)
		})
	})

	When("editing and restarting failed import", func() {
		It("should succeed", func() {
			runner.SetRemoteReaderClient(newRespondingReader())

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Completed))

			// log message timing is not guaranteed
			Eventually(func() string {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.Message
			}).Should(Equal("completed"))

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
				Message: "completed",
				State:   remoteread.TargetState_Completed,
			}

			AssertTargetStatus(expected, status)
			Expect(len(writerClient.Payloads)).To(Equal(1))
		})
	})

	When("target runner can reach target endpoint", func() {
		It("should complete", func() {
			runner.SetRemoteReaderClient(newRespondingReader())

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Completed))

			Eventually(func() string {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.Message
			}).Should(Equal("completed"))

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
				Message: "completed",
				State:   remoteread.TargetState_Completed,
			}

			AssertTargetStatus(expected, status)
			Expect(len(writerClient.Payloads)).To(Equal(1))
		})
	})

	When("target is stopped during push", func() {
		It("should be marked as stopped", func() {
			// new reader with the longest possible delay
			runner.SetRemoteReaderClient(newRespondingReader())
			runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
				return &mockRemoteWriteClient{
					Delay: math.MaxInt64,
				}
			}))

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Running))

			err = runner.Stop(target.Meta.Name)
			Expect(err).NotTo(HaveOccurred())

			// status dosn't update immediately so we need to extend the defualt timeout
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Canceled))
		})
	})

	When("target pushes with unrecoverable error", func() {
		It("should fail", func() {
			runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
				return &mockRemoteWriteClient{
					Error: fmt.Errorf("context canceled"),
				}
			}))
			runner.SetRemoteReaderClient(newRespondingReader())

			err := runner.Start(target, query)
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus
			Eventually(func() remoteread.TargetState {
				status, _ = runner.GetStatus(target.Meta.Name)
				return status.State
			}).Should(Equal(remoteread.TargetState_Failed))
		})
	})
})
