package metrics_test

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cortexproject/cortex/pkg/cortexpb"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"github.com/samber/lo"
	"go.uber.org/zap"
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

// readHttpHandler is only here to keep a remote reader connection open to keep it running indefinitely
type readHttpHandler struct {
	lg *zap.SugaredLogger
}

func NewReadHandler() http.Handler {
	return readHttpHandler{
		lg: logger.New(
			logger.WithLogLevel(zap.DebugLevel),
		).Named("read-handler"),
	}
}

func (h readHttpHandler) writeReadResponse(w http.ResponseWriter, r *prompb.ReadResponse) {
	uncompressed, err := proto.Marshal(r)
	if err != nil {
		panic(err)
	}

	compressed := snappy.Encode(nil, uncompressed)

	_, err = w.Write(compressed)
	if err != nil {
		panic(err)
	}
}

func (h readHttpHandler) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/block":
		// select {} will block forever without using CPU.
		select {}
	case "/large":
		h.writeReadResponse(w, &prompb.ReadResponse{
			Results: []*prompb.QueryResult{
				{
					Timeseries: []*prompb.TimeSeries{
						{
							Labels: []prompb.Label{
								{
									Name:  "__name__",
									Value: "test_metric",
								},
							},
							// Samples: lo.Map(make([]prompb.Sample, 4194304), func(sample prompb.Sample, i int) prompb.Sample {
							Samples: lo.Map(make([]prompb.Sample, 65536), func(sample prompb.Sample, i int) prompb.Sample {
								sample.Timestamp = time.Now().UnixMilli()
								return sample
							}),
						},
					},
				},
			},
		})
	case "/small":
		h.writeReadResponse(w, &prompb.ReadResponse{
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
		})
	case "/health":
	default:
		panic(fmt.Sprintf("unsupported endpoint: %s", request.URL.Path))
	}
}
