package metrics_test

import (
	"context"
	"fmt"
	"time"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func AssertTargetProgress(expected *remoteread.TargetProgress, actual *remoteread.TargetProgress) {
	GinkgoHelper()
	if expected == nil {
		Expect(actual).To(BeNil())
		return
	}
	Expect(actual.StartTimestamp.String()).To(Equal(expected.StartTimestamp.String()))
	Expect(actual.LastReadTimestamp.String()).To(Equal(expected.LastReadTimestamp.String()))
	Expect(actual.EndTimestamp.String()).To(Equal(expected.EndTimestamp.String()))
}

func AssertTargetStatus(expected *remoteread.TargetStatus, actual *remoteread.TargetStatus) {
	GinkgoHelper()
	// check message first so ginkgo will show us the error message
	Expect(actual.Message).To(Equal(expected.Message))
	Expect(actual.State).To(Equal(expected.State))
	AssertTargetProgress(expected.Progress, actual.Progress)
}

type mockRemoteReader struct {
	Error     error
	Responses []*prompb.ReadResponse

	// Delay is the time.Duration to wait before returning the next response.
	Delay time.Duration
	i     int
}

func (reader *mockRemoteReader) Read(_ context.Context, _ string, _ *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	if reader.Error != nil {
		return nil, reader.Error
	}

	if reader.i >= len(reader.Responses) {
		return nil, fmt.Errorf("all reader responses have alaredy been consumed")
	}

	time.Sleep(reader.Delay)

	response := reader.Responses[reader.i]
	reader.i++
	return response, reader.Error
}

type mockRemoteWriteClient struct {
	Error    error
	Payloads []*cortexpb.WriteRequest

	// Delay is the time.Duration to wait before returning.
	Delay time.Duration
}

func (client *mockRemoteWriteClient) Push(ctx context.Context, in *cortexpb.WriteRequest, _ ...grpc.CallOption) (*cortexpb.WriteResponse, error) {
	if client.Error != nil {
		return nil, client.Error
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(client.Delay):
	}

	client.Payloads = append(client.Payloads, in)
	return &cortexpb.WriteResponse{}, nil
}

func (client *mockRemoteWriteClient) SyncRules(_ context.Context, _ *remotewrite.Payload, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}
