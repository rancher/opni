package shipper_test

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	otlplogsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
)

func TestShipper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "shipper suite")
}

type mockLogsServiceServer struct {
	collogspb.UnsafeLogsServiceServer
	logs []*otlplogsv1.ResourceLogs
}

func newMockLogsServiceServer() *mockLogsServiceServer {
	return &mockLogsServiceServer{
		logs: []*otlplogsv1.ResourceLogs{},
	}
}

func (m *mockLogsServiceServer) Export(
	_ context.Context,
	req *collogspb.ExportLogsServiceRequest,
) (*collogspb.ExportLogsServiceResponse, error) {
	m.logs = req.GetResourceLogs()
	return &collogspb.ExportLogsServiceResponse{}, nil
}

type mockDateParser struct {
	timestamp time.Time
}

func (m *mockDateParser) ParseTimestamp(log string) (time.Time, string, bool) {
	return m.timestamp, log, true
}
