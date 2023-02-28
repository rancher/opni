package _import

import (
	"context"
	"fmt"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/plugins/import/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

func AssertTargetProgress(expected *remoteread.TargetProgress, actual *remoteread.TargetProgress) {
	if expected == nil {
		Expect(actual).To(BeNil())
		return
	}

	Expect(actual.StartTimestamp.String()).To(Equal(expected.StartTimestamp.String()))
	Expect(actual.LastReadTimestamp.String()).To(Equal(expected.LastReadTimestamp.String()))
	Expect(actual.EndTimestamp.String()).To(Equal(expected.EndTimestamp.String()))
}

func AssertTargetStatus(expected *remoteread.TargetStatus, actual *remoteread.TargetStatus) {
	// check message first so ginkgo will show us the error message
	Expect(actual.Message).To(Equal(expected.Message))
	Expect(actual.State).To(Equal(expected.State))
	AssertTargetProgress(expected.Progress, actual.Progress)
}

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
