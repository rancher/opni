package agent

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/http"
	"strings"
	"sync"
	"time"
)

// todo: this should probably be more sophisticated than this to handle read size limits
var TimeDeltaMillis = time.Minute.Milliseconds()

// todo: import prometheus LabelMatcher into plugins/metrics/pkg/apis/remoteread.proto to remove this
func toLabelMatchers(rrLabelMatchers []*remoteread.LabelMatcher) []*prompb.LabelMatcher {
	pbLabelMatchers := make([]*prompb.LabelMatcher, 0, len(rrLabelMatchers))

	for _, matcher := range rrLabelMatchers {
		var matchType prompb.LabelMatcher_Type

		switch matcher.Type {
		case remoteread.LabelMatcher_Equal:
			matchType = prompb.LabelMatcher_EQ
		case remoteread.LabelMatcher_NotEqual:
			matchType = prompb.LabelMatcher_NEQ
		case remoteread.LabelMatcher_RegexEqual:
			matchType = prompb.LabelMatcher_RE
		case remoteread.LabelMatcher_NotRegexEqual:
			matchType = prompb.LabelMatcher_NRE
		default:
			// todo: log something
		}

		pbLabelMatchers = append(pbLabelMatchers, &prompb.LabelMatcher{
			Type:  matchType,
			Name:  matcher.Name,
			Value: matcher.Value,
		})
	}

	return pbLabelMatchers
}

func dereferenceResultTimeseries(in []*prompb.TimeSeries) []prompb.TimeSeries {
	dereferenced := make([]prompb.TimeSeries, 0, len(in))

	for _, ref := range in {
		dereferenced = append(dereferenced, *ref)
	}

	return dereferenced
}

type Run struct {
	logger     *zap.SugaredLogger
	internalMu sync.RWMutex

	stopChan chan interface{}
	target   *remoteread.Target
	query    *remoteread.Query
}

func (run *Run) failed(message string) {
	run.internalMu.Lock()
	defer run.internalMu.Unlock()

	run.target.Status.State = remoteread.TargetStatus_Failed
	run.target.Status.Message = message
}

func (run *Run) running() {
	run.internalMu.Lock()
	defer run.internalMu.Unlock()

	run.target.Status.State = remoteread.TargetStatus_Running
	run.target.Status.Message = ""
}

func (run *Run) complete() {
	run.internalMu.Lock()
	defer run.internalMu.Unlock()

	run.target.Status.State = remoteread.TargetStatus_Complete
}

func (run *Run) stopped() {
	run.internalMu.Lock()
	defer run.internalMu.Unlock()

	run.target.Status.State = remoteread.TargetStatus_Stopped
}

func (run *Run) updateLastRead(lastReadSec int64) {
	run.internalMu.Lock()
	defer run.internalMu.Unlock()

	run.target.Status.Progress.LastReadTimestamp = timestamppb.New(time.UnixMilli(lastReadSec))
}

func (run *Run) updateStatus(fn func(status *remoteread.TargetStatus)) {
	run.internalMu.Lock()
	defer run.internalMu.Unlock()

	if run.target.Status == nil {
		run.target.Status = &remoteread.TargetStatus{}
	}

	fn(run.target.Status)
}

func (run *Run) getStatus() *remoteread.TargetStatus {
	run.internalMu.Lock()
	defer run.internalMu.Unlock()

	status := *run.target.Status

	return &status
}

// todo: put back under TargetRunner
func (run *Run) run(
	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient],
	remoteReader clients.Locker[RemoteReader],
) {
	labelMatchers := toLabelMatchers(run.query.Matchers)

	importEnd := run.query.EndTimestamp.AsTime().UnixMilli()
	nextStart := run.query.StartTimestamp.AsTime().UnixMilli()
	nextEnd := nextStart

	for nextStart < importEnd && run.getStatus().State == remoteread.TargetStatus_Running {
		nextStart = nextEnd
		nextEnd = nextStart + TimeDeltaMillis

		if nextStart >= importEnd {
			break
		}

		if nextEnd >= importEnd {
			nextEnd = importEnd
		}

		readRequest := &prompb.ReadRequest{
			Queries: []*prompb.Query{
				{
					StartTimestampMs: nextStart,
					EndTimestampMs:   nextEnd,
					Matchers:         labelMatchers,
				},
			},
		}

		var readResponse *prompb.ReadResponse
		var err error
		remoteReader.Use(func(client RemoteReader) {
			readResponse, err = client.Read(context.Background(), run.target.Spec.Endpoint, readRequest)
		})

		if err != nil {
			run.failed(fmt.Sprintf("failed to read from target endpoint: %s", err.Error()))
			return
		}

		for _, result := range readResponse.Results {
			if len(result.Timeseries) == 0 {
				continue
			}

			writeRequest := prompb.WriteRequest{
				Timeseries: dereferenceResultTimeseries(result.Timeseries),
			}

			uncompressed, err := proto.Marshal(&writeRequest)
			if err != nil {
				run.failed(fmt.Sprintf("failed to uncompress data from target endpoint: %s", err.Error()))
				return
			}

			compressed := snappy.Encode(nil, uncompressed)

			payload := &remotewrite.Payload{
				Contents: compressed,
			}

			remoteWriteClient.Use(func(remoteWriteClient remotewrite.RemoteWriteClient) {
				if _, err := remoteWriteClient.Push(context.Background(), payload); err != nil {
					run.failed("failed to push to remote write")
					return
				}

				run.updateLastRead(nextEnd)
			})

			run.logger.With(
				"cluster", run.target.Meta.ClusterId,
				"target", run.target.Meta.Name,
			).Debugf("pushed remote write payload: %s", payload.String())
		}

		run.updateLastRead(nextEnd)
	}

	if run.getStatus().State == remoteread.TargetStatus_Running {
		run.logger.With(
			"cluster", run.target.Meta.ClusterId,
			"target", run.target.Meta.Name,
		).Infof("run completed")

		run.complete()
	}
}

type TargetRunner interface {
	Start(target *remoteread.Target, query *remoteread.Query) error

	Stop(name string) error

	GetStatus(name string) (*remoteread.TargetStatus, error)

	SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient])

	SetRemoteReader(client clients.Locker[RemoteReader])
}

func NewTargetRunner(logger *zap.SugaredLogger) TargetRunner {
	return &targetRunner{
		logger: logger.Named("runner"),
		runsMu: sync.RWMutex{},
		runs:   make(map[string]*Run),
		remoteReader: clients.NewLocker(nil, func(grpc.ClientConnInterface) RemoteReader {
			return NewRemoteReader(&http.Client{})
		}),
	}
}

type targetRunner struct {
	logger *zap.SugaredLogger

	runsMu sync.RWMutex
	runs   map[string]*Run

	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient]
	remoteReader      clients.Locker[RemoteReader]
}

func (runner *targetRunner) Start(target *remoteread.Target, query *remoteread.Query) error {
	runner.runsMu.Lock()
	defer runner.runsMu.Unlock()

	run, found := runner.runs[target.Meta.Name]
	if found {
		status := run.getStatus()
		if status.State == remoteread.TargetStatus_Running {
			return fmt.Errorf("target is already running")
		}
	}

	newRun := &Run{
		logger:     runner.logger.Named(target.Meta.Name),
		internalMu: sync.RWMutex{},
		stopChan:   make(chan interface{}),
		target:     target,
		query:      query,
	}

	// always replace pre-existing Run since Target might have been edited on the gateway
	runner.runs[newRun.target.Meta.Name] = newRun

	newRun.updateStatus(func(status *remoteread.TargetStatus) {
		status.Progress = &remoteread.TargetProgress{
			StartTimestamp:    newRun.query.StartTimestamp,
			LastReadTimestamp: newRun.query.StartTimestamp,
			EndTimestamp:      newRun.query.EndTimestamp,
		}
		status.Message = ""
		status.State = remoteread.TargetStatus_Running
	})

	go newRun.run(runner.remoteWriteClient, runner.remoteReader)

	return nil
}

func (runner *targetRunner) Stop(name string) error {
	runner.runsMu.Lock()
	defer runner.runsMu.Unlock()

	run, found := runner.runs[name]

	if found {
		return fmt.Errorf("target is not running")
	}

	close(run.stopChan)
	run.stopped()

	delete(runner.runs, name)

	return nil
}

func (runner *targetRunner) GetStatus(name string) (*remoteread.TargetStatus, error) {
	runner.runsMu.RLock()
	defer runner.runsMu.RUnlock()

	run, found := runner.runs[name]
	if !found {
		return nil, fmt.Errorf("target not found")
	}

	return run.getStatus(), nil
}

func (runner *targetRunner) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	runner.remoteWriteClient = client
}

func (runner *targetRunner) SetRemoteReader(client clients.Locker[RemoteReader]) {
	runner.remoteReader = client
}

func mapKeys[T any](m map[string]T) string {
	l := make([]string, 0, len(m))
	for k, _ := range m {
		l = append(l, k)
	}

	return strings.Join(l, ", ")
}
