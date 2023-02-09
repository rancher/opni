package agent

import (
	"context"
	"fmt"
	
	// todo: needed instead of google.golang.org/protobuf/proto since prometheus Messages are built with it
	"github.com/golang/protobuf/proto"

	"github.com/golang/snappy"
	"github.com/opentracing-contrib/go-stdlib/nethttp"
	promConfig "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sync"
	"time"
)

func targetIsRunningError(name string) error {
	return fmt.Errorf("target '%s' is running, cannot be removed, modified, or started", name)
}

func targetIsNotRunningError(name string) error {
	return fmt.Errorf("target '%s' is not running", name)
}

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
	stopChan chan interface{}
	target   *remoteread.Target
	query    *remoteread.Query
}

func (run *Run) failed(message string) {
	run.target.Status.State = remoteread.TargetStatus_Failed
	run.target.Status.Message = message
}

func (run *Run) running() {
	run.target.Status.State = remoteread.TargetStatus_Running
	run.target.Status.Message = ""
}

func (run *Run) complete() {
	run.target.Status.State = remoteread.TargetStatus_Complete
}

func (run *Run) stopped() {
	run.target.Status.State = remoteread.TargetStatus_Stopped
}

func (run *Run) updateLastRead(lastReadSec int64) {
	run.target.Status.Progress.LastReadTimestamp = timestamppb.New(time.UnixMilli(lastReadSec))
}

// todo: add context

type TargetRunner interface {
	Start(target *remoteread.Target, query *remoteread.Query) error

	Stop(name string) error

	GetStatus(name string) (*remoteread.TargetStatus, error)

	SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient])
}

func NewTargetRunner(logger *zap.SugaredLogger) TargetRunner {
	return &targetRunner{
		logger: logger,
		runs:   make(map[string]Run),
	}
}

type targetRunner struct {
	logger *zap.SugaredLogger

	runsMu sync.RWMutex
	runs   map[string]Run

	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient]
	remoteReadClient  clients.Locker[remoteread.RemoteReadGatewayClient]
}

func (runner *targetRunner) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	runner.remoteWriteClient = client
}

func (runner *targetRunner) SetRemoteReadClient(client clients.Locker[remoteread.RemoteReadGatewayClient]) {
	runner.remoteReadClient = client
}

func (runner *targetRunner) run(run Run, remoteReaderClient *RemoteReaderClient) {
	runner.runsMu.Lock()
	runner.runs[run.target.Meta.Name] = run
	runner.runsMu.Unlock()

	labelMatchers := toLabelMatchers(run.query.Matchers)

	// todo: this should probably be more sophisticated than this to handle read size limits
	importEnd := run.query.EndTimestamp.AsTime().UnixMilli()
	nextEndDelta := time.Minute.Milliseconds() * 5

	nextStart := run.query.StartTimestamp.AsTime().UnixMilli()
	nextEnd := nextStart

	run.target.Status = &remoteread.TargetStatus{
		Progress: &remoteread.TargetProgress{
			StartTimestamp: run.query.StartTimestamp,
			EndTimestamp:   run.query.EndTimestamp,
		},
		Message: "",
		State:   remoteread.TargetStatus_Running,
	}

	defer func() {
		// todo: defer this stuff
		if run.target.Status.State == remoteread.TargetStatus_Running {
			runner.logger.With(
				"cluster", run.target.Meta.ClusterId,
				"target", run.target.Meta.Name,
			).Infof("run completed")

			run.complete()
		}
	}()

	for nextStart < importEnd && run.target.Status.State == remoteread.TargetStatus_Running {
		nextStart = nextEnd
		nextEnd = nextStart + nextEndDelta
		if nextEnd > importEnd {
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

		readResponse, err := remoteReaderClient.Read(context.TODO(), run.target.Spec.Endpoint, readRequest)

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

			runner.remoteWriteClient.Use(func(remoteWriteClient remotewrite.RemoteWriteClient) {
				if _, err := remoteWriteClient.Push(context.TODO(), payload); err != nil {
					run.failed("failed to push to remote write")
					return
				}

				run.updateLastRead(nextEnd)
			})

			runner.logger.With(
				"cluster", run.target.Meta.ClusterId,
				"target", run.target.Meta.Name,
			).Infof("pushed remote write payload: %s", payload.String())
		}

		run.updateLastRead(nextEnd)
	}
}

func (runner *targetRunner) Start(target *remoteread.Target, query *remoteread.Query) error {
	// We want to allow for restarting a Failed or Completed. We should not encounter NotRunning, Stopped, or Completed.
	run, found := runner.runs[target.Meta.Name]
	if found && run.target.Status.State == remoteread.TargetStatus_Running {
		switch run.target.Status.State {
		case remoteread.TargetStatus_Running:
			return targetIsRunningError(target.Meta.Name)
		default:
			runner.logger.With(
				"cluster", target.Meta.ClusterId,
				"target", target.Meta.Name,
				"old state", target.Status.State,
			).Warnf("restarting target")
		}
	} else if !found {
		run = Run{
			stopChan: make(chan interface{}),
			target:   target,
			query:    query,
		}
	}

	prometheusClient, err := promConfig.NewClientFromConfig(promConfig.HTTPClientConfig{}, fmt.Sprintf("%s-remoteread", run.target.Meta.Name), promConfig.WithHTTP2Disabled())
	if err != nil {
		return fmt.Errorf("could not start import: %w", err)
	}

	prometheusClient.Transport = &nethttp.Transport{
		RoundTripper: prometheusClient.Transport,
	}

	remoteReaderClient := NewRemoteReaderClient(run.stopChan, prometheusClient)

	go runner.run(run, remoteReaderClient)

	runner.logger.With(
		"cluster", target.Meta.ClusterId,
		"name", target.Meta.Name,
	).Infof("target started")

	return nil
}

func (runner *targetRunner) Stop(name string) error {
	run, found := runner.runs[name]

	if !found {
		return targetIsNotRunningError(name)
	}

	close(run.stopChan)
	delete(runner.runs, name)

	run.stopped()

	runner.logger.With(
		"cluster", run.target.Meta.ClusterId,
		"name", run.target.Meta.Name,
	).Infof("target stopped")

	return nil
}

func (runner *targetRunner) GetStatus(name string) (*remoteread.TargetStatus, error) {
	runner.runsMu.Lock()
	run, found := runner.runs[name]
	runner.runsMu.Unlock()

	if !found {
		return nil, targetIsNotRunningError(name)
	}

	return run.target.Status, nil
}
