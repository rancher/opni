package task

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sync"
	"time"

	"github.com/qmuntal/stateless"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/rancher/opni/pkg/logger"
	"github.com/rancher/opni/pkg/storage"
	"github.com/rancher/opni/pkg/util"
)

// Controller manages launching tasks and configuring their underlying state
// machines, and saving their state to persistent storage.
type Controller struct {
	// this context is kept around so that we can run operations scoped to the
	// lifetime of the controller (e.g. interacting with the kv store)
	// inside tasks that have been canceled.
	ctx     context.Context
	name    string
	store   KVStore
	locks   util.LockMap[string, *sync.Mutex]
	runner  TaskRunner
	logger  *zap.SugaredLogger
	tasks   map[string]context.CancelFunc
	tasksMu sync.Mutex
}

// Creates a new task controller, and resumes any tasks that have state saved
// in the key-value store if they did not already complete. Saved state for
// completed tasks will be cleaned.
func NewController(ctx context.Context, name string, store KVStore, runner TaskRunner) (*Controller, error) {
	ctrl := &Controller{
		ctx:    ctx,
		name:   name,
		store:  storage.NewKeyValueStoreWithPrefix(store, fmt.Sprintf("/tasks/%s/", name)),
		locks:  util.NewLockMap[string, *sync.Mutex](),
		runner: runner,
		logger: logger.New().Named("tasks"),
		tasks:  make(map[string]context.CancelFunc),
	}
	existingTasks, err := ctrl.ListTasks()
	if err != nil {
		return nil, err
	}
	for _, id := range existingTasks {
		for {
			status, err := ctrl.TaskStatus(id)
			if err != nil {
				if util.StatusCode(err) == codes.NotFound {
					break
				}
				ctrl.logger.With(
					zap.String("id", id),
					zap.Error(err),
				).Warn("failed to look up task (will retry)")
				time.Sleep(time.Second)
				continue
			}
			switch status.State {
			case StatePending, StateRunning:
				if err := ctrl.LaunchTask(id, WithJsonMetadata(status.GetMetadata())); err != nil {
					ctrl.logger.With(
						zap.String("id", id),
						zap.Error(err),
					).Warn("failed to resume task (will retry)")
					time.Sleep(time.Second)
					continue
				}
			case StateCompleted, StateFailed, StateCanceled:
				// clean up the old task state
				rw := storage.NewValueStoreLocker(storage.NewValueStore(ctrl.store, id), ctrl.locks.Get(id))
				rw.Lock()
				if err := rw.Delete(ctx); err != nil {
					ctrl.logger.With(
						zap.String("id", id),
						zap.Error(err),
					).Warn("failed to clean task state")
					// continue anyway, this will be retried next time
					// the controller is started
				}
				rw.Unlock()
			}
			break
		}
	}
	return ctrl, nil
}

type NewTaskOptions struct {
	metadata       any
	jsonMetadata   *string
	statusCallback chan *Status
	stateCallback  chan State
}

type NewTaskOption func(*NewTaskOptions)

func (o *NewTaskOptions) apply(opts ...NewTaskOption) {
	for _, op := range opts {
		op(o)
	}
}

func WithMetadata(md any) NewTaskOption {
	// ensure md is a struct type
	if reflect.TypeOf(md).Kind() != reflect.Struct {
		panic("metadata must be a json-encodable struct")
	}

	return func(o *NewTaskOptions) {
		o.metadata = md
	}
}

func WithJsonMetadata(md string) NewTaskOption {
	return func(o *NewTaskOptions) {
		o.jsonMetadata = &md
	}
}

// When the task enters one of the completed states (Completed, Failed, or
// Canceled), the task's status will be written to the provided channel.
// If the channel cannot be written to at the time the task is completed, the
// status will be dropped, so a buffered channel should be used if necessary.
func WithStatusCallback(ch chan *Status) NewTaskOption {
	return func(o *NewTaskOptions) {
		o.statusCallback = ch
	}
}

// Like StatusCallback, but only writes the task's state value instead of the
// entire status.
func WithStateCallback(ch chan State) NewTaskOption {
	return func(o *NewTaskOptions) {
		o.stateCallback = ch
	}
}

func (c *Controller) LaunchTask(id string, opts ...NewTaskOption) error {
	if err := c.ctx.Err(); err != nil {
		return fmt.Errorf("cannot launch task: %w (controller context)", err)
	}

	options := &NewTaskOptions{}
	options.apply(opts...)

	var mdJson string
	if options.jsonMetadata != nil {
		mdJson = *options.jsonMetadata
	} else if options.metadata != nil {
		data, err := json.Marshal(options.metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		mdJson = string(data)
	}

	c.tasksMu.Lock()
	defer c.tasksMu.Unlock()

	if cancel, ok := c.tasks[id]; ok {
		cancel()
	}

	taskCtx, taskCancel := context.WithCancel(c.ctx)

	rw := storage.NewValueStoreLocker(storage.NewValueStore(c.store, id), c.locks.Get(id))

	accessor := func(taskCtx context.Context) (interface{}, error) {
		rw.Lock()
		defer rw.Unlock()

		status, err := rw.Get(c.ctx)
		if err != nil {
			return nil, err
		}
		return status.State, nil
	}
	mutator := func(taskCtx context.Context, state interface{}) error {
		rw.Lock()
		defer rw.Unlock()

		ts, err := rw.Get(c.ctx)
		if err != nil {
			if util.StatusCode(err) == codes.NotFound {
				ts = &corev1.TaskStatus{
					State:    state.(corev1.TaskState),
					Metadata: mdJson,
				}
			} else {
				return err
			}
		}
		ts.State = state.(corev1.TaskState)
		return rw.Put(c.ctx, ts)
	}

	t := &Task{
		id:     id,
		cctx:   c.ctx,
		status: rw,
		logger: c.logger.With(zap.String("id", id)),
	}

	sm := stateless.NewStateMachineWithExternalStorage(accessor, mutator, stateless.FiringQueued)

	sm.SetTriggerParameters(corev1.TaskTrigger_Error, reflect.TypeOf((*error)(nil)).Elem())
	sm.OnTransitioning(func(ctx context.Context, transition stateless.Transition) {
		// applies to all OnEntry handlers, but not OnActive
		t.logTransition(&corev1.StateTransition{
			State:     transition.Destination.(corev1.TaskState),
			Timestamp: timestamppb.Now(),
		})
	})
	sm.Configure(corev1.TaskState_Pending).
		Permit(corev1.TaskTrigger_Start, corev1.TaskState_Running).
		Permit(corev1.TaskTrigger_Cancel, corev1.TaskState_Canceled).
		OnActive(func(taskCtx context.Context) error {
			err := c.runner.OnTaskPending(taskCtx, t)
			if err != nil {
				if taskCtx.Err() != nil && taskCtx.Err() == err {
					return sm.Fire(corev1.TaskTrigger_Cancel)
				}
				return sm.Fire(corev1.TaskTrigger_Error, err)
			}
			return sm.FireCtx(taskCtx, corev1.TaskTrigger_Start)
		})
	sm.Configure(corev1.TaskState_Running).
		Permit(corev1.TaskTrigger_End, corev1.TaskState_Completed).
		Permit(corev1.TaskTrigger_Cancel, corev1.TaskState_Canceled).
		Permit(corev1.TaskTrigger_Error, corev1.TaskState_Failed).
		OnEntry(func(taskCtx context.Context, _ ...any) error {
			err := c.runner.OnTaskRunning(taskCtx, t)
			if err != nil {
				if taskCtx.Err() != nil && taskCtx.Err() == err {
					return sm.Fire(corev1.TaskTrigger_Cancel)
				}
				return sm.Fire(corev1.TaskTrigger_Error, err)
			}
			return sm.Fire(corev1.TaskTrigger_End)
		})

	maybeSendCallbacks := func() {
		t.status.Lock()
		status := t.getStatus()
		t.status.Unlock()
		if options.statusCallback != nil {
			select {
			case options.statusCallback <- status:
			default:
			}
		}
		if options.stateCallback != nil {
			select {
			case options.stateCallback <- status.GetState():
			default:
			}
		}
	}

	sm.Configure(corev1.TaskState_Completed).
		OnEntry(func(taskCtx context.Context, args ...any) error {
			c.runner.OnTaskCompleted(taskCtx, t, corev1.TaskState_Completed, args)
			maybeSendCallbacks()
			return nil
		})
	sm.Configure(corev1.TaskState_Failed).
		OnEntry(func(taskCtx context.Context, args ...any) error {
			c.runner.OnTaskCompleted(taskCtx, t, corev1.TaskState_Failed, args)
			maybeSendCallbacks()
			return nil
		})
	sm.Configure(corev1.TaskState_Canceled).
		OnEntry(func(taskCtx context.Context, args ...any) error {
			c.runner.OnTaskCompleted(taskCtx, t, corev1.TaskState_Canceled, args)
			maybeSendCallbacks()
			return nil
		})

	c.tasks[t.id] = taskCancel

	// If there is no previously saved state, set the state to pending
	state, err := accessor(taskCtx)
	if err != nil {
		if util.StatusCode(err) == codes.NotFound {
			if err := mutator(taskCtx, corev1.TaskState_Pending); err != nil {
				return err
			}
		}
	} else {
		// When restarting tasks, some values are kept from the previous saved
		// status and some are reset or cleared, based on the task's last known
		// state, according to the following table (State is always reset to
		// Pending regardless of the previous state):
		// ┌───────────┬─────────────┬─────────────┬─────────────┬─────────────┐
		// │     State │ Progress    │ Metadata    │ Logs        │ Transitions │
		// ├───────────┼─────────────┼─────────────┼─────────────┼─────────────┤
		// │   Running │ Keep        │ Keep        │ Keep        │ Keep        │
		// │    Failed │ Clear       │ Reset       │ Keep+Append │ Keep        │
		// │  Canceled │ Keep        │ Reset       │ Clear       │ Clear       │
		// ├ ─ ─ ─ ─ ─ ┼ ─ ─ ─ ─ ─ ─ ┼ ─ ─ ─ ─ ─ ─ ┼ ─ ─ ─ ─ ─ ─ ┼ ─ ─ ─ ─ ─ ─ ┤
		// │   Unknown │ Clear       │ Reset       │ Clear       │ Clear       │
		// │   Pending │ Clear       │ Reset       │ Clear       │ Clear       │
		// │ Completed │ Clear       │ Reset       │ Clear       │ Clear       │
		// └───────────┴─────────────┴─────────────┴─────────────┴─────────────┘
		rw.Lock()
		prev, err := rw.Get(c.ctx)
		if err != nil {
			rw.Unlock()
			return err
		}
		prev.State = corev1.TaskState_Pending
		switch state.(corev1.TaskState) {
		case corev1.TaskState_Running:
		case corev1.TaskState_Failed:
			prev.Progress = nil
			prev.Metadata = mdJson
			prev.Logs = append(prev.Logs, &corev1.LogEntry{
				Msg:       fmt.Sprintf("internal: restarting task (previous state: %s)", state),
				Level:     int32(zapcore.InfoLevel),
				Timestamp: timestamppb.Now(),
			})
		case corev1.TaskState_Canceled:
			prev.Metadata = mdJson
			prev.Logs = nil
			prev.Transitions = nil
		default:
			prev.Progress = nil
			prev.Metadata = mdJson
			prev.Logs = nil
			prev.Transitions = nil
		}
		err = rw.Put(c.ctx, prev)
		rw.Unlock()
		if err != nil {
			return err
		}
	}

	t.logTransition(&corev1.StateTransition{
		State:     corev1.TaskState_Pending,
		Timestamp: timestamppb.Now(),
	})

	go sm.ActivateCtx(taskCtx)

	return nil
}

func (c *Controller) ListTasks() ([]string, error) {
	keys, err := c.store.ListKeys(c.ctx, "")
	if err != nil {
		return nil, err
	}
	// only take the last part of the key, which is the task id
	return lo.Map(keys, util.Indexed(path.Base)), nil
}

func (c *Controller) TaskStatus(id string) (*corev1.TaskStatus, error) {
	rw := storage.NewValueStoreLocker(storage.NewValueStore(c.store, id), c.locks.Get(id))
	rw.Lock()
	defer rw.Unlock()
	status, err := rw.Get(c.ctx)
	if err != nil {
		return nil, err
	}
	return util.ProtoClone(status), nil
}

// Attempts a best-effort cancellation of a task. There are no guarantees that
// the task will actually be canceled. If a task is already completed, this
// has no effect.
func (c *Controller) CancelTask(id string) {
	c.tasksMu.Lock()
	defer c.tasksMu.Unlock()
	cancel, ok := c.tasks[id]
	if ok && cancel != nil {
		cancel()
	}
	delete(c.tasks, id)
}
