package shipper

import (
	"bufio"
	"context"
	"runtime"
	"strings"
	"sync"

	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/adapter"
	"github.com/open-telemetry/opentelemetry-collector-contrib/pkg/stanza/entry"
	"github.com/rancher/opni/pkg/supportagent/dateparser"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type otlpShipper struct {
	otlpShipperOptions
	client                 collogspb.LogsServiceClient
	dateParser             dateparser.DateParser
	converter              *adapter.Converter
	collectedErrorMessages []string
	failureCount           int
	readingDone            chan struct{}
	wg                     sync.WaitGroup
	lg                     *zap.SugaredLogger
}

type otlpShipperOptions struct {
	component string
	logType   string
	batchSize int
	workers   int
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

func WithWorkers(workers int) otlpShipperOption {
	return func(o *otlpShipperOptions) {
		o.workers = workers
	}
}

func NewOTLPShipper(
	cc grpc.ClientConnInterface,
	parser dateparser.DateParser,
	lg *zap.SugaredLogger,
	opts ...otlpShipperOption,
) Shipper {
	options := otlpShipperOptions{
		batchSize: 50,
		workers:   runtime.GOMAXPROCS(0),
	}
	options.apply(opts...)
	return &otlpShipper{
		otlpShipperOptions: options,
		client:             collogspb.NewLogsServiceClient(cc),
		dateParser:         parser,
		converter:          adapter.NewConverter(lg.Desugar()),
		readingDone:        make(chan struct{}),
		lg:                 lg,
	}
}

func (s *otlpShipper) Publish(ctx context.Context, tokens *bufio.Scanner) error {
	s.converter.Start()
	defer s.converter.Stop()
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.shipLogs(ctx)
	}

	continueScan := tokens.Scan()
	var previousEnt *entry.Entry
	entries := make([]*entry.Entry, 0, s.batchSize)
	for continueScan {
		line := tokens.Text()
		datetime, log, valid := s.dateParser.ParseTimestamp(line)
		if valid {
			// If this log is valid the previous one is ready to be shipped
			if previousEnt != nil {
				previousEnt.AddAttribute("log", previousEnt.Body.(string))
				entries = append(entries, previousEnt)
			}

			if len(entries) >= s.batchSize {
				s.lg.Infof("batching %d logs", len(entries))

				err := s.converter.Batch(entries)
				if err != nil {
					s.lg.Errorw("failed to batch logs", zap.Error(err))
					s.failureCount += len(entries)
					s.collectedErrorMessages = append(s.collectedErrorMessages, err.Error())
				}
				entries = make([]*entry.Entry, 0, s.batchSize)
			}

			ent := s.newEntry()
			ent.Timestamp = datetime
			ent.Body = log
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

	if err := tokens.Err(); err != nil {
		s.lg.Errorw("failed to scan logs", zap.Error(err))
	}

	close(s.readingDone)
	// wait for shipping to finish
	s.lg.Info("waiting for shipping to finish")
	s.wg.Wait()
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
	defer s.wg.Done()
	for {
		select {
		case <-ctx.Done():
			s.lg.Info("context cancelled, stopping shipping")
			return
		case logs := <-s.converter.OutChannel():
			s.exportLogs(ctx, logs)
		default:
			select {
			case <-s.readingDone:
				return
			default:
			}
		}
	}
}

// TODO: should use a backoff queue for this.
func (s *otlpShipper) exportLogs(ctx context.Context, logs plog.Logs) {
	exporRequest := plogotlp.NewExportRequestFromLogs(logs)
	protoBytes, err := exporRequest.MarshalProto()
	if err != nil {
		s.lg.Error("failed to marshal proto")
		s.collectedErrorMessages = append(s.collectedErrorMessages, err.Error())
		return
	}

	req := &collogspb.ExportLogsServiceRequest{}
	err = proto.Unmarshal(protoBytes, req)
	if err != nil {
		s.lg.Error("failed to unmarshal proto")
		s.collectedErrorMessages = append(s.collectedErrorMessages, err.Error())
		return
	}

	resp, err := s.client.Export(ctx, req)
	if err != nil {
		s.lg.With("error", err).Error("failed to ship logs")
		s.collectedErrorMessages = append(s.collectedErrorMessages, err.Error())
		s.failureCount += len(req.GetResourceLogs())
		return
	}
	if resp.GetPartialSuccess().GetRejectedLogRecords() > 0 {
		s.collectedErrorMessages = append(s.collectedErrorMessages, resp.GetPartialSuccess().GetErrorMessage())
	}
	s.lg.Infof("shipped %d batch", len(req.GetResourceLogs()))
}

func (s *otlpShipper) newEntry() *entry.Entry {
	ent := entry.New()
	if s.component != "" {
		ent.AddResourceKey("kubernetes_component", s.component)
	}
	ent.AddResourceKey("log_type", s.logType)
	return ent
}
