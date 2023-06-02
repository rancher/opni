package shipper

import (
	"bufio"
	"context"
	"strings"
	"unsafe"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
	"github.com/rancher/opni/pkg/supportagent/dateparser"
	"go.opentelemetry.io/collector/pdata/plog"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

type otlpShipper struct {
	otlpShipperOptions
	client                 collogspb.LogsServiceClient
	dateParser             dateparser.DateParser
	converter              *adapter.Converter
	collectedErrorMessages []string
	failureCount           int
	shippingFinished       chan struct{}
	lg                     *zap.SugaredLogger
}

type otlpShipperOptions struct {
	component string
	logType   string
	batchSize int
}

type otlpShipperOption func(*otlpShipperOptions)

func (o *otlpShipperOptions) apply(opts ...otlpShipperOption) {
	for _, opt := range opts {
		opt(o)
	}
}

func WithComponent(component string) otlpShipperOption {
	return func(o *otlpShipperOptions) {
		o.component = component
	}
}

func WithLogType(logType string) otlpShipperOption {
	return func(o *otlpShipperOptions) {
		o.logType = logType
	}
}

func WithBatchSize(batchSize int) otlpShipperOption {
	return func(o *otlpShipperOptions) {
		o.batchSize = batchSize
	}
}

type convertLogs struct {
	orig *collogspb.ExportLogsServiceRequest
}

func NewOTLPShipper(
	cc grpc.ClientConnInterface,
	parser dateparser.DateParser,
	lg *zap.SugaredLogger,
	opts ...otlpShipperOption,
) Shipper {
	options := otlpShipperOptions{
		batchSize: 20,
	}
	options.apply(opts...)
	return &otlpShipper{
		otlpShipperOptions: options,
		client:             collogspb.NewLogsServiceClient(cc),
		dateParser:         parser,
		converter:          adapter.NewConverter(lg.Desugar()),
		shippingFinished:   make(chan struct{}),
		lg:                 lg,
	}
}

func (s *otlpShipper) Publish(ctx context.Context, tokens *bufio.Scanner) error {
	s.converter.Start()
	go s.shipLogs(ctx)

	continueScan := tokens.Scan()
	var previousEnt *entry.Entry
	entries := make([]*entry.Entry, 0, s.batchSize)
	for continueScan {
		line := tokens.Text()
		datetime, log, valid := s.dateParser.ParseTimestamp(line)
		if valid {
			ent := s.newEntry()
			ent.Timestamp = datetime
			ent.Body = log

			if len(entries) >= s.batchSize {
				err := s.converter.Batch(entries)
				if err != nil {
					s.lg.Errorw("failed to batch logs", zap.Error(err))
					s.failureCount += len(entries)
					s.collectedErrorMessages = append(s.collectedErrorMessages, err.Error())
				}
				entries = make([]*entry.Entry, 0, s.batchSize)
			}
			entries = append(entries, ent)
			previousEnt = ent
		} else {
			if previousEnt == nil {
				s.lg.Warn("failed to parse first timestamp, skipping line")
			} else {
				previousLine := previousEnt.Body.(string)
				previousEnt.Body = previousLine + "\n" + line
			}
		}

		continueScan = tokens.Scan()
	}

	if len(entries) > 0 {
		err := s.converter.Batch(entries)
		if err != nil {
			s.lg.Errorw("failed to batch logs", zap.Error(err))
			s.failureCount += len(entries)
			s.collectedErrorMessages = append(s.collectedErrorMessages, err.Error())
		}
	}

	s.converter.Stop()

	if err := tokens.Err(); err != nil {
		s.lg.Errorw("failed to scan logs", zap.Error(err))
	}

	// wait for shipping to finish
	<-s.shippingFinished
	if s.failureCount > 0 {
		s.lg.Errorf("failed to ship %d logs for log type %s", s.failureCount, s.logType)
		if s.component != "" {
			s.lg.Errorf("failed component was %s", s.component)
		}
		s.lg.Error(strings.Join(s.collectedErrorMessages, "\n"))
	}
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

func (s *otlpShipper) newEntry() *entry.Entry {
	ent := entry.New()
	if s.component != "" {
		ent.AddResourceKey("kubernetes_component", s.component)
	}
	ent.AddResourceKey("log_type", s.logType)
	return ent
}

func requestFromLogs(logs *plog.Logs) *collogspb.ExportLogsServiceRequest {
	l := *(*convertLogs)(unsafe.Pointer(logs))
	return l.orig
}
