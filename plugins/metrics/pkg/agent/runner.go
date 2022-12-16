package agent

import (
	"context"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/opentracing-contrib/go-stdlib/nethttp"
	promConfig "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/prompb"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"time"
)

func targetAlreadyExistsError(name string) error {
	return fmt.Errorf("target '%s' already exists", name)
}

func targetDoesNotExistError(name string) error {
	return fmt.Errorf("target '%s' does not exist", name)
}

func targetIsRunningError(name string) error {
	return fmt.Errorf("target '%s' is running, cannot be removed, modified, or started", name)
}

func targetIsNotRunningError(name string) error {
	return fmt.Errorf("target '%s' is not running", name)
}

// todo: import prometheus LabelMatcher into apis/remoteread.proto to remove this
func toLabelMatchers(rrLabelMatchers []*remoteread.LabelMatcher) []*prompb.LabelMatcher {
	pbLabelMatchers := make([]*prompb.LabelMatcher, 0, len(rrLabelMatchers))

	for _, matcher := range rrLabelMatchers {
		var matchType prompb.LabelMatcher_Type

		switch matcher.Type {
		case remoteread.LabelMatcher_EQUAL:
			matchType = prompb.LabelMatcher_EQ
		case remoteread.LabelMatcher_NOT_EQUAL:
			matchType = prompb.LabelMatcher_NEQ
		case remoteread.LabelMatcher_REGEX_EQUAL:
			matchType = prompb.LabelMatcher_RE
		case remoteread.LabelMatcher_NOT_REGEX_EQUAL:
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

type TargetRunner interface {
	Add(target *remoteread.Target) error

	Edit(name string, diff *remoteread.TargetDiff) error

	Remove(name string) error

	List() *remoteread.TargetList

	Start(name string, query *remoteread.Query) error

	Stop(name string) error

	SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient])
}

func NewTargetRunner() TargetRunner {
	return &targetRunner{
		targets: make(map[string]*remoteread.Target),
		runs:    make(map[string]Run),
	}
}

type targetRunner struct {
	targets map[string]*remoteread.Target
	runs    map[string]Run

	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient]
}

func (runner *targetRunner) Add(target *remoteread.Target) error {
	if _, found := runner.targets[target.Name]; found {
		return targetAlreadyExistsError(target.Name)
	}

	runner.targets[target.Name] = target

	return nil
}

func (runner *targetRunner) Edit(name string, diff *remoteread.TargetDiff) error {
	target, found := runner.targets[name]

	if !found {
		return targetDoesNotExistError(target.Name)
	}

	if _, found := runner.runs[name]; found {
		return targetIsRunningError(name)
	}

	if diff.Endpoint != "" {
		target.Endpoint = diff.Endpoint
	}

	if diff.Name != "" {
		target.Name = diff.Name

		delete(runner.targets, name)
	}

	runner.targets[target.Name] = target

	return nil
}

func (runner *targetRunner) Remove(name string) error {
	if _, found := runner.targets[name]; !found {
		return targetDoesNotExistError(name)
	}

	delete(runner.targets, name)

	return nil
}

func (runner *targetRunner) List() *remoteread.TargetList {
	targets := make([]*remoteread.Target, 0, len(runner.targets))

	for _, target := range runner.targets {
		targets = append(targets, target)
	}

	return &remoteread.TargetList{Targets: targets}
}

func (runner *targetRunner) run(run Run, remoteReadClient *RemoteReaderClient) {
	labelMatchers := toLabelMatchers(run.query.Matchers)

	// todo: this should probably be a lot more sophisticated than this
	importEnd := run.query.EndTimestamp.AsTime().UnixMilli()
	nextEndDelta := time.Minute.Milliseconds() * 5

	nextStart := run.query.StartTimestamp.AsTime().UnixMilli()

	for nextStart < importEnd {
		nextEnd := nextStart + nextEndDelta
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

		readResponse, err := remoteReadClient.Read(context.TODO(), run.target.Endpoint, readRequest)

		if err != nil {
			// todo: log this event
			return
		}

		for _, result := range readResponse.Results {
			writeRequest := prompb.WriteRequest{
				Timeseries: dereferenceResultTimeseries(result.Timeseries),
			}

			uncompressed, err := proto.Marshal(&writeRequest)
			if err != nil {
				// todo: log failure
				continue
			}

			compressed := snappy.Encode(nil, uncompressed)

			payload := &remotewrite.Payload{
				Contents: compressed,
			}

			runner.remoteWriteClient.Use(func(remoteWriteClient remotewrite.RemoteWriteClient) {
				if _, err := remoteWriteClient.Push(context.TODO(), payload); err != nil {
					// todo: log this error
				}
			})
		}
	}
}

func (runner *targetRunner) Start(name string, query *remoteread.Query) error {
	if _, found := runner.runs[name]; found {
		return targetIsRunningError(name)
	}

	target, found := runner.targets[name]

	if !found {
		return targetDoesNotExistError(name)
	}

	run := Run{
		stopChan: make(chan interface{}),
		target:   target,
		query:    query,
	}

	prometheusClient, err := promConfig.NewClientFromConfig(promConfig.HTTPClientConfig{}, fmt.Sprintf("%s-remoteread", run.target.Name), promConfig.WithHTTP2Disabled())
	if err != nil {
		return fmt.Errorf("could not start import: %w", err)
	}

	prometheusClient.Transport = &nethttp.Transport{
		RoundTripper: prometheusClient.Transport,
	}

	remoteReadClient := NewRemoteReadClient(run.stopChan, prometheusClient)

	go runner.run(run, remoteReadClient)
	runner.runs[name] = run

	return nil
}

func (runner *targetRunner) Stop(name string) error {
	run, found := runner.runs[name]

	if !found {
		return targetIsNotRunningError(name)
	}

	close(run.stopChan)
	delete(runner.runs, name)

	return nil
}

func (runner *targetRunner) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	runner.remoteWriteClient = client
}
