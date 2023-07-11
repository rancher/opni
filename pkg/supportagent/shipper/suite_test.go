package shipper_test

import (
	"context"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	otlplogsv1 "go.opentelemetry.io/proto/otlp/logs/v1"

	_ "github.com/rancher/opni/pkg/test/setup"
)

func TestShipper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "shipper suite")
}

type mockLogsServiceServer struct {
	collogspb.UnsafeLogsServiceServer
	logs   []*otlplogsv1.ResourceLogs
	muLogs sync.RWMutex
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
	m.muLogs.Lock()
	defer m.muLogs.Unlock()
	m.logs = req.GetResourceLogs()
	return &collogspb.ExportLogsServiceResponse{}, nil
}

func (m *mockLogsServiceServer) getLogs() []*otlplogsv1.ResourceLogs {
	m.muLogs.RLock()
	defer m.muLogs.RUnlock()
	return m.logs
}

type mockDateParser struct {
	timestamp time.Time
}

func (m *mockDateParser) ParseTimestamp(log string) (time.Time, string, bool) {
	return m.timestamp, log, true
}
