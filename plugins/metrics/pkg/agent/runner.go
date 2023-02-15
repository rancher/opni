package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/pkg/apis/remotewrite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
	"strings"
	"sync"
	"time"
)

var TimeDeltaMillis = time.Minute.Milliseconds()

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
			panic(fmt.Sprintf("bug: bad matcher type %d", matcher.Type))
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

func getMessageFromTaskLogs(logs []*corev1.LogEntry) string {
	for i := len(logs) - 1; i >= 0; i-- {
		log := logs[i]
		if log.Level == int32(zapcore.ErrorLevel) {
			return log.Msg
		}
	}

	return ""
}

type TargetRunMetadata struct {
	Target *remoteread.Target
	Query  *remoteread.Query
}

type targetStore struct {
	innerMu sync.RWMutex
	inner   map[string]*corev1.TaskStatus
}

func (store *targetStore) Put(_ context.Context, key string, value *corev1.TaskStatus) error {
	store.innerMu.Lock()
	defer store.innerMu.Unlock()
	store.inner[key] = value
	return nil
}

func (store *targetStore) Get(_ context.Context, key string) (*corev1.TaskStatus, error) {
	store.innerMu.RLock()
	defer store.innerMu.RUnlock()

	status, found := store.inner[key]
	if !found {
		return nil, storage.ErrNotFound
	}

	return status, nil
}

func (store *targetStore) Delete(_ context.Context, key string) error {
	store.innerMu.Lock()
	defer store.innerMu.Unlock()
	delete(store.inner, key)
	return nil
}

func (store *targetStore) ListKeys(_ context.Context, prefix string) ([]string, error) {
	store.innerMu.RLock()
	defer store.innerMu.RUnlock()
	keys := make([]string, 0, len(store.inner))
	for key := range store.inner {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

type taskRunner struct {
	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient]

	remoteReaderMu sync.RWMutex
	remoteReader   RemoteReader
}

func (tr *taskRunner) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	tr.remoteWriteClient = client
}

func (tr *taskRunner) SetRemoteReaderClient(client RemoteReader) {
	tr.remoteReaderMu.Lock()
	defer tr.remoteReaderMu.Unlock()

	tr.remoteReader = client
}

func (tr *taskRunner) OnTaskPending(_ context.Context, _ task.ActiveTask) error {
	return nil
}

func (tr *taskRunner) OnTaskRunning(_ context.Context, activeTask task.ActiveTask) error {
	run := &TargetRunMetadata{}
	activeTask.LoadTaskMetadata(run)

	labelMatchers := toLabelMatchers(run.Query.Matchers)

	importEnd := run.Query.EndTimestamp.AsTime().UnixMilli()
	nextStart := run.Query.StartTimestamp.AsTime().UnixMilli()
	nextEnd := nextStart

	progress := &corev1.Progress{
		Current: 0,
		Total:   uint64(importEnd - nextStart),
	}

	activeTask.SetProgress(progress)

	for nextStart < importEnd {
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

		tr.remoteReaderMu.Lock()
		readResponse, err := tr.remoteReader.Read(context.Background(), run.Target.Spec.Endpoint, readRequest)
		tr.remoteReaderMu.Unlock()

		if err != nil {
			activeTask.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("failed to read from target endpoint: %s", err))
			return fmt.Errorf("failed to read from target endpoint: %w", err)
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
				activeTask.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("failed to uncompress data from target endpoint: %s", err))
				return fmt.Errorf("failed to uncompress data from target endpoint: %w", err)
			}

			compressed := snappy.Encode(nil, uncompressed)

			payload := &remotewrite.Payload{
				Contents: compressed,
			}

			tr.remoteWriteClient.Use(func(remoteWriteClient remotewrite.RemoteWriteClient) {
				if _, err := remoteWriteClient.Push(context.Background(), payload); err != nil {
					activeTask.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("failed to push to remote write: %s", err))
					return
				}

				activeTask.AddLogEntry(zapcore.DebugLevel, fmt.Sprintf("pushed %d bytes to remote write", len(payload.Contents)))
			})

			progress.Current = uint64(nextEnd)
			activeTask.SetProgress(progress)
		}
	}

	return nil
}

func (tr *taskRunner) OnTaskCompleted(_ context.Context, activeTask task.ActiveTask, state task.State, _ ...any) {
	switch state {
	case task.StateCompleted:
		activeTask.AddLogEntry(zapcore.InfoLevel, "completed")
	case task.StateFailed:
		// a log will be added in OnTaskRunning for failed imports so we don't need to log anything here
	case task.StateCanceled:
		activeTask.AddLogEntry(zapcore.WarnLevel, "canceled")
	}
}

type TargetRunner interface {
	Start(target *remoteread.Target, query *remoteread.Query) error
	Stop(name string) error
	GetStatus(name string) (*remoteread.TargetStatus, error)
	SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient])
	SetRemoteReaderClient(client RemoteReader)
}

type taskingTargetRunner struct {
	logger *zap.SugaredLogger

	runnerMu sync.RWMutex
	runner   *taskRunner

	controller *task.Controller

	remoteWriteClient clients.Locker[remotewrite.RemoteWriteClient]
}

func NewTargetRunner(logger *zap.SugaredLogger) TargetRunner {
	store := &targetStore{
		inner: make(map[string]*corev1.TaskStatus),
	}

	runner := &taskRunner{}

	controller, err := task.NewController(context.Background(), "target-runner", store, runner)
	if err != nil {
		panic(fmt.Sprintf("bug: failed to create target task controller: %s", err))
	}

	return &taskingTargetRunner{
		logger:     logger,
		runner:     runner,
		controller: controller,
	}
}

func (runner *taskingTargetRunner) Start(target *remoteread.Target, query *remoteread.Query) error {
	if status, err := runner.controller.TaskStatus(target.Meta.Name); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("error checking for target status: %s", err)
		}
	} else {
		switch status.State {
		case corev1.TaskState_Running, corev1.TaskState_Pending:
			return fmt.Errorf("target is already running")
		case corev1.TaskState_Unknown:
			return fmt.Errorf("target state is unknown")
		}
	}

	if err := runner.controller.LaunchTask(target.Meta.Name, task.WithMetadata(TargetRunMetadata{
		Target: target,
		Query:  query,
	})); err != nil {
		return fmt.Errorf("could not run target: %w", err)
	}

	runner.logger.Infof("started target '%s'", target.Meta.Name)

	return nil
}

func (runner *taskingTargetRunner) Stop(name string) error {
	status, err := runner.controller.TaskStatus(name)
	if err != nil {
		return fmt.Errorf("target not found")
	}

	switch status.State {
	case corev1.TaskState_Canceled, corev1.TaskState_Completed, corev1.TaskState_Failed:
		return fmt.Errorf("target is not running")
	}

	runner.controller.CancelTask(name)

	runner.logger.Infof("stopped target '%s'", name)

	return nil
}

func (runner *taskingTargetRunner) GetStatus(name string) (*remoteread.TargetStatus, error) {
	taskStatus, err := runner.controller.TaskStatus(name)

	if err != nil {
		if util.StatusCode(err) == codes.NotFound {
			return &remoteread.TargetStatus{
				State: remoteread.TargetState_NotRunning,
			}, nil
		}

		return nil, fmt.Errorf("could not get target status: %w", err)
	}

	taskMetadata := TargetRunMetadata{}
	if err := json.Unmarshal([]byte(taskStatus.Metadata), &taskMetadata); err != nil {
		return nil, fmt.Errorf("could not parse target metedata: %w", err)
	}

	statusProgress := &remoteread.TargetProgress{
		StartTimestamp:    taskMetadata.Query.StartTimestamp,
		LastReadTimestamp: nil,
		EndTimestamp:      taskMetadata.Query.EndTimestamp,
	}

	if taskStatus.Progress == nil {
		statusProgress.LastReadTimestamp = statusProgress.StartTimestamp
	} else {
		statusProgress.LastReadTimestamp = &timestamppb.Timestamp{
			// progress is stored in milliseconds, so we need to convert
			Seconds: statusProgress.StartTimestamp.Seconds + int64(taskStatus.Progress.Current/1000),
		}
	}

	var state remoteread.TargetState
	switch taskStatus.State {
	case corev1.TaskState_Unknown:
		state = remoteread.TargetState_Unknown
	case corev1.TaskState_Pending, corev1.TaskState_Running:
		state = remoteread.TargetState_Running
	case corev1.TaskState_Completed:
		state = remoteread.TargetState_Completed
	case corev1.TaskState_Failed:
		state = remoteread.TargetState_Failed
	case corev1.TaskState_Canceled:
		state = remoteread.TargetState_Canceled
	}

	return &remoteread.TargetStatus{
		Progress: statusProgress,
		Message:  getMessageFromTaskLogs(taskStatus.Logs),
		State:    state,
	}, nil
}

func (runner *taskingTargetRunner) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	runner.runner.SetRemoteWriteClient(client)
}

func (runner *taskingTargetRunner) SetRemoteReaderClient(client RemoteReader) {
	runner.runnerMu.Lock()
	defer runner.runnerMu.Unlock()

	runner.runner.SetRemoteReaderClient(client)
}
