package agent

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/util/wait"
	"strings"
	"time"
)

type mockRemoteReader struct {
	Error     error
	Responses []*prompb.ReadResponse
	i         int
}

func (reader *mockRemoteReader) Read(ctx context.Context, endpoint string, readRequest *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	if reader.Error != nil {
		return nil, reader.Error
	}

	if reader.i >= len(reader.Responses) {
		return nil, fmt.Errorf("all reader responses have alaredy been consumed")
	}

	response := reader.Responses[reader.i]
	reader.i++
	return response, reader.Error
}

type mockRemoteWriteClient struct {
	Payloads []*remotewrite.Payload
}

func (client *mockRemoteWriteClient) Push(ctx context.Context, in *remotewrite.Payload, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	client.Payloads = append(client.Payloads, in)
	return &emptypb.Empty{}, nil
}

func (client *mockRemoteWriteClient) SyncRules(ctx context.Context, in *remotewrite.Payload, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

type mockRemoteReadGatewayClient struct {
}

func (client *mockRemoteReadGatewayClient) AddTarget(ctx context.Context, in *remoteread.TargetAddRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

func (client *mockRemoteReadGatewayClient) EditTarget(ctx context.Context, in *remoteread.TargetEditRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

func (client *mockRemoteReadGatewayClient) RemoveTarget(ctx context.Context, in *remoteread.TargetRemoveRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

func (client *mockRemoteReadGatewayClient) ListTargets(ctx context.Context, in *remoteread.TargetListRequest, opts ...grpc.CallOption) (*remoteread.TargetList, error) {
	return nil, nil
}

func (client *mockRemoteReadGatewayClient) Start(ctx context.Context, in *remoteread.StartReadRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

func (client *mockRemoteReadGatewayClient) Stop(ctx context.Context, in *remoteread.StopReadRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

func (client *mockRemoteReadGatewayClient) GetTargetStatus(ctx context.Context, in *remoteread.TargetStatusRequest, opts ...grpc.CallOption) (*remoteread.TargetStatus, error) {
	return nil, nil
}

func (client *mockRemoteReadGatewayClient) Discover(ctx context.Context, in *remoteread.DiscoveryRequest, opts ...grpc.CallOption) (*remoteread.DiscoveryResponse, error) {
	return nil, nil
}

var _ = Describe("Target Runner", Ordered, Label("integration"), func() {
	var (
		runner TargetRunner
		target *remoteread.Target

		writerClient *mockRemoteWriteClient
		readClient   *mockRemoteReadGatewayClient
		remoteReader *mockRemoteReader
	)

	BeforeEach(func() {
		lg := logger.NewPluginLogger().Named("test-runner")

		writerClient = &mockRemoteWriteClient{}
		readClient = &mockRemoteReadGatewayClient{}
		remoteReader = &mockRemoteReader{
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

		runner = NewTargetRunner(lg)
		runner.SetRemoteWriteClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remotewrite.RemoteWriteClient {
			return writerClient
		}))
		runner.SetRemoteReadClient(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) remoteread.RemoteReadGatewayClient {
			return readClient
		}))
		runner.SetRemoteReader(clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) RemoteReader {
			return remoteReader
		}))

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
	})

	When("target runner cannot reach target endpoint", func() {
		It("should fail", func() {
			remoteReader := clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) RemoteReader {
				return &mockRemoteReader{
					Error: fmt.Errorf("failed"),
				}
			})
			runner.SetRemoteReader(remoteReader)

			err := runner.Start(
				target,
				&remoteread.Query{
					StartTimestamp: &timestamppb.Timestamp{},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
					},
					Matchers: nil,
				})
			Expect(err).NotTo(HaveOccurred())

			var status *remoteread.TargetStatus

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			wait.UntilWithContext(ctx, func(context.Context) {
				status, err = runner.GetStatus(target.Meta.Name)
				if status != nil && status.State == remoteread.TargetStatus_Failed {
					cancel()
				}
			}, time.Millisecond*100)

			if err := ctx.Err(); err != nil && !strings.Contains(err.Error(), "context canceled") {
				Fail("waiting for expected state failed")
			}

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp:    &timestamppb.Timestamp{},
					LastReadTimestamp: nil,
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "failed to read from target endpoint: failed",
				State:   remoteread.TargetStatus_Failed,
			}

			Expect(expected).To(Equal(status))

			remoteReader = clients.NewLocker(nil, func(connInterface grpc.ClientConnInterface) RemoteReader {
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
			})
			runner.SetRemoteReader(remoteReader)

			// todo: test restarting the target
			err = runner.Start(
				target,
				&remoteread.Query{
					StartTimestamp: &timestamppb.Timestamp{},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
					},
					Matchers: nil,
				})
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel = context.WithTimeout(context.Background(), time.Second)
			wait.UntilWithContext(ctx, func(context.Context) {
				status, err = runner.GetStatus(target.Meta.Name)

				Expect(err).ToNot(HaveOccurred())

				if status.State == remoteread.TargetStatus_Complete {
					cancel()
				}
			}, time.Millisecond*100)

			if err := ctx.Err(); err != nil && !strings.Contains(err.Error(), "context canceled") {
				Fail(fmt.Sprintf("waiting for expected state failed: %s", err))
			}

			expected = &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp: &timestamppb.Timestamp{},
					LastReadTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "",
				State:   remoteread.TargetStatus_Complete,
			}

			Expect(expected).To(Equal(status))
			Expect(len(writerClient.Payloads)).To(Equal(1))
		})
	})

	When("target runner can reach target endpoint", func() {
		It("should complete", func() {
			err := runner.Start(
				target,
				&remoteread.Query{
					StartTimestamp: &timestamppb.Timestamp{},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(), // ensures only 1 import cycle will occur
					},
					Matchers: nil,
				})
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			var status *remoteread.TargetStatus

			wait.UntilWithContext(ctx, func(context.Context) {
				status, err = runner.GetStatus(target.Meta.Name)

				Expect(err).ToNot(HaveOccurred())

				if status.State == remoteread.TargetStatus_Complete {
					cancel()
				}
			}, time.Millisecond*100)

			if err := ctx.Err(); err != nil && !strings.Contains(err.Error(), "context canceled") {
				Fail(fmt.Sprintf("waiting for expected state failed: %s", err))
			}

			expected := &remoteread.TargetStatus{
				Progress: &remoteread.TargetProgress{
					StartTimestamp: &timestamppb.Timestamp{},
					LastReadTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
					EndTimestamp: &timestamppb.Timestamp{
						Seconds: TimeDeltaMillis / 2 / time.Second.Milliseconds(),
					},
				},
				Message: "",
				State:   remoteread.TargetStatus_Complete,
			}

			Expect(expected).To(Equal(status))
			Expect(len(writerClient.Payloads)).To(Equal(1))
		})
	})
})
