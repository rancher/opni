package task

import (
	"context"
	"log/slog"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/storage"
)

type (
	State  = corev1.TaskState
	Status = corev1.TaskStatus

	KVStore = storage.KeyValueStoreT[*corev1.TaskStatus]
)

type (
	stateAccessor = func(ctx context.Context) (any, error)
	stateMutator  = func(context.Context, any) error
)

const (
	StateUnknown   = corev1.TaskState_Unknown
	StatePending   = corev1.TaskState_Pending
	StateRunning   = corev1.TaskState_Running
	StateCompleted = corev1.TaskState_Completed
	StateFailed    = corev1.TaskState_Failed
	StateCanceled  = corev1.TaskState_Canceled
)

// ActiveTask is an interface to a currently running task's state and metadata.
// All functions in this interface other than TaskId() will block and retry
// upon encountering an error (e.g. storage unavailable), or until the task
// controller's context is canceled.
type ActiveTask interface {
	// Returns the task's ID.
	TaskId() string
	// Decodes the task's metadata (json string) into the output variable.
	// The output variable must be a pointer to a struct. If the task has no
	// metadata, the output variable will be left unchanged.
	LoadTaskMetadata(output any)
	// Sets the task's progress.
	SetProgress(progress *corev1.Progress)
	// Gets the task's progress.
	GetProgress() *corev1.Progress
	// Adds a log entry to the task's status.
	AddLogEntry(level slog.Level, msg string)
}

type TaskRunner interface {
	// Handles the Pending state of a task. The task will remain Pending until
	// this function returns.
	// The state of the task is transitioned depending on the return value:
	// - If the function returns nil, the task will be transitioned to Running.
	// - If the function returns an error other than ctx.Err(), the task will be
	//   transitioned to Failed.
	// - If the function returns ctx.Err(), the task will be transitioned to
	//   Canceled.
	OnTaskPending(ctx context.Context, ti ActiveTask) error

	// Handles the Running state of a task. The task will remain Running until
	// this function returns.
	// The state of the task is transitioned depending on the return value:
	// - If the function returns nil, the task will be transitioned to Completed.
	// - If the function returns an error other than ctx.Err(), the task will be
	//   transitioned to Failed.
	// - If the function returns ctx.Err(), the task will be transitioned to
	//   Canceled. Implementations should be careful not to return ctx.Err()
	//   unless the task can actually be resumed. Resumed tasks will have their
	//   status persisted, so any cancelable task implementation should always
	//   check the task's status as soon as this function is called.
	OnTaskRunning(ctx context.Context, ti ActiveTask) error

	// Called when a task is moved to one of several end states - Completed,
	// Failed, or Canceled.
	OnTaskCompleted(ctx context.Context, ti ActiveTask, state State, args ...any)
}
