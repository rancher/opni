package logging

import (
	"context"

	"github.com/rancher/opni/pkg/machinery/uninstall"
	"github.com/rancher/opni/pkg/task"
)

type UninstallTaskRunner struct {
	uninstall.DefaultPendingHandler
}

func (a *UninstallTaskRunner) OnTaskRunning(ctx context.Context, ti task.ActiveTask) error {
	panic("not implemented")
}

func (a *UninstallTaskRunner) OnTaskCompleted(ctx context.Context, ti task.ActiveTask, state task.State, args ...any) {
	panic("not implemented")
}
