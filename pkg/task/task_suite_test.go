package task_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/task"
	_ "github.com/rancher/opni/pkg/test/setup"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap/zapcore"
)

func TestTask(t *testing.T) {
	// SetDefaultEventuallyTimeout(1 * time.Hour)
	RegisterFailHandler(Fail)
	RunSpecs(t, "Task Suite")
}

var ctrl *gomock.Controller

var _ = BeforeSuite(func() {
	ctrl = gomock.NewController(GinkgoT())
})

type SampleTaskConfig struct {
	// Number of items to read from the input channel to complete the task
	Limit int `json:"limit"`
}

type SampleTaskRunner struct {
	// Tasks read items from this channel
	Input chan any
}

func (a *SampleTaskRunner) OnTaskPending(ctx context.Context, ti task.ActiveTask) error {
	ti.AddLogEntry(zapcore.DebugLevel, "pending")
	return ctx.Err()
}

func (a *SampleTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	var config SampleTaskConfig
	ti.LoadTaskMetadata(&config)

	ti.AddLogEntry(zapcore.InfoLevel, "running")

	progress := ti.GetProgress()
	for i := progress.GetCurrent(); i < uint64(config.Limit); i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-a.Input:
			if !ok {
				return errors.New("input channel closed")
			}
		}
		ti.SetProgress(&corev1.Progress{
			Current: i + 1,
			Total:   uint64(config.Limit),
		})
	}

	return nil
}

func (a *SampleTaskRunner) OnTaskCompleted(_ context.Context, ti task.ActiveTask, state task.State, _ ...any) {
	switch state {
	case task.StateCompleted:
		ti.AddLogEntry(zapcore.InfoLevel, "completed")
	case task.StateFailed:
		ti.AddLogEntry(zapcore.ErrorLevel, "failed")
	case task.StateCanceled:
		ti.AddLogEntry(zapcore.WarnLevel, "canceled")
	}
}
