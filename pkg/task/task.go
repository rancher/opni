package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"log/slog"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage/kvutil"
	"github.com/rancher/opni/pkg/util"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Task struct {
	id     string
	cctx   context.Context // controller context
	status kvutil.ValueStoreLocker[*corev1.TaskStatus]
	logger *slog.Logger
}

func (t *Task) TaskId() string {
	return t.id
}

func (t *Task) LoadTaskMetadata(output any) {
	t.status.Lock()
	defer t.status.Unlock()
	status := t.getStatus()
	md := status.GetMetadata()
	if strings.TrimSpace(md) == "" {
		return
	}
	if err := json.Unmarshal([]byte(md), output); err != nil {
		panic(fmt.Sprintf("bug: failed to unmarshal task metadata into type %T: %v", output, err))
	}
}

func (t *Task) SetProgress(progress *corev1.Progress) {
	t.status.Lock()
	defer t.status.Unlock()
	status := t.getStatus()
	if status == nil {
		return
	}
	status.Progress = util.ProtoClone(progress)
	t.putStatus(status)
}

func (t *Task) GetProgress() *corev1.Progress {
	t.status.Lock()
	defer t.status.Unlock()
	return t.getStatus().GetProgress()
}

func (t *Task) AddLogEntry(level slog.Level, msg string) {
	t.status.Lock()
	defer t.status.Unlock()
	status := t.getStatus()
	if status == nil {
		return
	}
	switch level {
	case slog.LevelDebug:
		t.logger.Debug(msg)
	case slog.LevelInfo:
		t.logger.Info(msg)
	case slog.LevelWarn:
		t.logger.Warn(msg)
	case slog.LevelError:
		t.logger.Error(msg)
	default:
		t.logger.Info(msg)
	}
	status.Logs = append(status.Logs, &corev1.LogEntry{
		Msg:       msg,
		Level:     int32(level),
		Timestamp: timestamppb.Now(),
	})
	t.putStatus(status)
}

func (t *Task) logTransition(transition *corev1.StateTransition) {
	t.status.Lock()
	defer t.status.Unlock()
	status := t.getStatus()
	if status == nil {
		return
	}
	status.Transitions = append(status.Transitions, transition)
	t.putStatus(status)
}

// requires status lock held
func (t *Task) getStatus() *corev1.TaskStatus {
	for {
		select {
		case result := <-lo.Async2(func() (*corev1.TaskStatus, error) {
			return t.status.Get(t.cctx)
		}):
			status, err := lo.Unpack2(result)
			if err == nil {
				return status
			}
			t.logger.With(
				zap.Error(err),
			).Warn("failed to get task status (will retry)")
			time.Sleep(time.Second)
		case <-t.cctx.Done():
			return nil
		}
	}
}

// requires status lock held
func (t *Task) putStatus(status *corev1.TaskStatus) {
	for {
		select {
		case err := <-lo.Async(func() error { return t.status.Put(t.cctx, status) }):
			if err == nil {
				return
			}
			t.logger.With(
				zap.Error(err),
			).Warn("failed to set task status (will retry)")
			time.Sleep(time.Second)
		case <-t.cctx.Done():
			return
		}
	}
}
