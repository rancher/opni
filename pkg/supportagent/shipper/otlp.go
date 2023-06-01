package shipper

import (
	"bufio"
	"context"
	"unsafe"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/rancher/opni/pkg/supportagent/dateparser"
	"go.opentelemetry.io/collector/pdata/plog"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
)

type otlpShipper struct {
	client                 collogspb.LogsServiceClient
	dateParser             dateparser.DateParser
	converter              adapter.Converter
	collectedErrorMessages []string
	failureCount           int
	shippingFinished       chan<- struct{}
}

type convertLogs struct {
	orig *collogspb.ExportLogsServiceRequest
}

func NewOTLPShipper(cc grpc.ClientConnInterface, parser dateparser.DateParser) Shipper {
	return &otlpShipper{
		client:     collogspb.NewLogsServiceClient(cc),
		dateParser: parser,
	}
}

func (s *otlpShipper) Publish(ctx context.Context, tokens bufio.Scanner) error {
	s.converter.Start()
	go s.shipLogs(ctx)
	return nil
}

func (s *otlpShipper) shipLogs(ctx context.Context) {
	for logs := range s.converter.OutChannel() {
		// Have
		req := requestFromLogs(&logs)
		resp, err := s.client.Export(ctx, req)
		if err != nil {
			s.collectedErrorMessages = append(s.collectedErrorMessages, err.Error())
			s.failureCount += len(req.GetResourceLogs())
		}
		if resp.GetPartialSuccess().GetRejectedLogRecords() > 0 {
			s.collectedErrorMessages = append(s.collectedErrorMessages, resp.GetPartialSuccess().GetErrorMessage())
		}
	}
	close(s.shippingFinished)
}

func requestFromLogs(logs *plog.Logs) *collogspb.ExportLogsServiceRequest {
	l := *(*convertLogs)(unsafe.Pointer(logs))
	return l.orig
}
