package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/backoff/v2"
	"github.com/prometheus/prometheus/prompb"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/clients"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/task"
	"github.com/rancher/opni/pkg/util"
	"github.com/rancher/opni/pkg/util/waitctx"
	"github.com/rancher/opni/plugins/metrics/apis/remoteread"
	"github.com/rancher/opni/plugins/metrics/apis/remotewrite"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var TimeDeltaMillis = time.Minute.Milliseconds()

const BufferDir = "/var/lib/opni-agent/import-buffer"

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

func getMessageFromTaskLogs(logs []*corev1.LogEntry) string {
	if len(logs) == 0 {
		return ""
	}

	for i := len(logs) - 1; i >= 0; i-- {
		log := logs[i]
		if log.Level != int32(zapcore.DebugLevel) {
			return log.Msg
		}
	}

	return ""
}

type TargetRunMetadata struct {
	Target *remoteread.Target
	Query  *remoteread.Query
}

// todo: could probably find a better name for this
// todo: replace ProgressRatio and Query with a ProgressDelta
type WriteMetadata struct {
	Target     string
	Query      *prompb.Query
	WriteChunk *prompb.WriteRequest

	// ProgressRatio is the ratio of the progress of this chunk to the total progress of the original request.
	ProgressRatio float64
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

	backoffPolicy backoff.Policy

	logger *zap.SugaredLogger

	buffer Buffer[WriteMetadata]
}

func newTaskRunner(logger *zap.SugaredLogger) (*taskRunner, error) {
	buffer, err := NewDiskBuffer(BufferDir)
	if err != nil {
		err := fmt.Errorf("could not create buffer: %w", err)
		return nil, fmt.Errorf("could not create buffer: %w", err)
	}

	return &taskRunner{
		backoffPolicy: backoff.Exponential(
			backoff.WithMaxRetries(0),
			backoff.WithMinInterval(5*time.Second),
			backoff.WithMaxInterval(5*time.Minute),
			backoff.WithMultiplier(1.1),
		),
		logger: logger.Named("task-runner"),
		buffer: buffer,
	}, nil
}

func (tr *taskRunner) SetRemoteWriteClient(client clients.Locker[remotewrite.RemoteWriteClient]) {
	tr.remoteWriteClient = client
}

func (tr *taskRunner) OnTaskPending(_ context.Context, _ task.ActiveTask) error {
	return nil
}

func (tr *taskRunner) doPush(ctx context.Context, writeRequest *prompb.WriteRequest) error {
	expbackoff := tr.backoffPolicy.Start(ctx)

	for {
		select {
		case <-expbackoff.Done():
			return ctx.Err()
		case <-expbackoff.Next():
			var err error

			tr.remoteWriteClient.Use(func(remoteWriteClient remotewrite.RemoteWriteClient) {
				promClient := remotewrite.AsPrometheusRemoteWriteClient(remoteWriteClient)
				_, err = promClient.Push(ctx, writeRequest)
			})

			if err == nil {
				return nil
			}

			// if task context is cancelled, return immediately
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			switch {
			case strings.Contains(err.Error(), "ingestion rate limit"):
				tr.logger.With(
					zap.Error(err),
				).Warn("failed to push to remote write, retrying...")
			default:
				return fmt.Errorf("failed to push to remote write: %w", err)
			}
		}
	}
}

func (tr *taskRunner) doRead(ctx context.Context, reader RemoteReader, run *TargetRunMetadata, readRequest *prompb.ReadRequest) (*prompb.ReadResponse, error) {
	expbackoff := tr.backoffPolicy.Start(ctx)

	for {
		select {
		case <-expbackoff.Done():
			return nil, ctx.Err()
		case <-expbackoff.Next():
			readResponse, err := reader.Read(context.Background(), run.Target.Spec.Endpoint, readRequest)

			if err == nil {
				return readResponse, nil
			}

			tr.logger.With(
				zap.Error(err),
				"endpoint", run.Target.Spec.Endpoint,
			).Warn("failed to read from target endpoint, retrying...")
		}
	}
}

func (tr *taskRunner) OnTaskRunning(ctx context.Context, activeTask task.ActiveTask) error {
	limit := util.DefaultWriteLimit()

	wc := waitctx.FromContext(ctx)

	run := &TargetRunMetadata{}
	activeTask.LoadTaskMetadata(run)

	var err error

	waitctx.Go(wc, func() {
		labelMatchers := toLabelMatchers(run.Query.Matchers)

		importEnd := run.Query.EndTimestamp.AsTime().UnixMilli()
		nextStart := run.Query.StartTimestamp.AsTime().UnixMilli()
		nextEnd := nextStart

		reader := NewRemoteReader(&http.Client{})

		for err == nil && nextStart < importEnd {
			select {
			case <-wc.Done():
				return
			default: // continue reading
			}

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

			readResponse, err = tr.doRead(wc, reader, run, readRequest)
			if err != nil {
				return
			}

			writeRequest := &prompb.WriteRequest{
				Timeseries: lo.Map(readResponse.Results[0].GetTimeseries(), func(t *prompb.TimeSeries, _ int) prompb.TimeSeries {
					return lo.FromPtr(t)
				}),
				Metadata: []prompb.MetricMetadata{},
			}

			var chunks []*prompb.WriteRequest

			chunks, err = util.SplitChunksWithLimit(writeRequest, limit)
			if err != nil {
				return
			}

			activeTask.AddLogEntry(zapcore.InfoLevel, fmt.Sprintf("split request into %d chunks", len(chunks)))

			lo.ForEach(chunks, func(chunk *prompb.WriteRequest, i int) {
				if err := tr.buffer.Add(wc, WriteMetadata{
					Target:        run.Target.Meta.Name,
					Query:         readRequest.Queries[0],
					WriteChunk:    chunk,
					ProgressRatio: 1.0 / float64(len(chunks)),
				}); err != nil {
					activeTask.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("could not add chunk to buffer: %s", err.Error()))
				}
			})
		}
	})

	waitctx.Go(wc, func() {
		progress := &corev1.Progress{
			Current: 0,
			Total:   uint64(run.Query.EndTimestamp.AsTime().UnixMilli() - run.Query.StartTimestamp.AsTime().UnixMilli()),
		}
		activeTask.SetProgress(progress)

		for err == nil && progress.Current < progress.Total {
			select {
			case <-wc.Done():
				return
			default: // continue pushing
			}

			var meta WriteMetadata

			meta, err = tr.buffer.Get(wc)
			if err != nil {
				activeTask.AddLogEntry(zapcore.ErrorLevel, fmt.Sprintf("could not get chunk from buffer: %s", err.Error()))
				continue
			}

			activeTask.AddLogEntry(zapcore.DebugLevel, "received chunk from buffer")

			if err = tr.doPush(wc, meta.WriteChunk); err != nil {
				return
			}

			progressDelta := uint64(float64(meta.Query.EndTimestampMs-meta.Query.StartTimestampMs) * meta.ProgressRatio)

			progress.Current += progressDelta
			activeTask.SetProgress(progress)
		}
	})

	waitctx.Wait(wc)

	if err != nil {
		activeTask.AddLogEntry(zapcore.ErrorLevel, err.Error())
		return err
	}

	return ctx.Err()
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
}

type taskingTargetRunner struct {
	logger *zap.SugaredLogger

	runnerMu sync.RWMutex
	runner   *taskRunner

	controller *task.Controller
}

func NewTargetRunner(logger *zap.SugaredLogger) TargetRunner {
	store := &targetStore{
		inner: make(map[string]*corev1.TaskStatus),
	}

	runner, err := newTaskRunner(logger)
	if err != nil {
		panic(fmt.Sprintf("bug: failed to create target task runner: %s", err))
	}

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
	runner.runnerMu.Lock()
	defer runner.runnerMu.Unlock()

	runner.runner.SetRemoteWriteClient(client)
}
