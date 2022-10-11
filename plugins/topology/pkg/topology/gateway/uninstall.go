package gateway

import (
	"context"
	"github.com/rancher/opni/pkg/task"
)

type NoOpUninstallTaskRunner struct {
}

func (n NoOpUninstallTaskRunner) OnTaskPending(ctx context.Context, ti task.ActiveTask) error {
	//TODO implement me
	panic("implement me")
}

func (n NoOpUninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	//TODO implement me
	panic("implement me")
}

func (n NoOpUninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
	//TODO implement me
	panic("implement me")
}

var _ task.TaskRunner = (*NoOpUninstallTaskRunner)(nil)
