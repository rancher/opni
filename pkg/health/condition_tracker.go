package health

import (
	"fmt"
	"sync"
	"time"

	"log/slog"

	gsync "github.com/kralicky/gpkg/sync"
	"go.uber.org/atomic"
)

type ConditionStatus int32

const (
	StatusPending ConditionStatus = iota
	StatusFailure
	StatusDisabled
)

func (s ConditionStatus) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusFailure:
		return "Failure"
	case StatusDisabled:
		return "Disabled"
	}
	return ""
}

func ConditionStatusStrToEnum(status string) (ConditionStatus, error) {
	switch status {
	case "Pending":
		return StatusPending, nil
	case "Failure":
		return StatusFailure, nil
	case "Disabled":
		return StatusDisabled, nil
	}
	return 0, fmt.Errorf("unknown condition status %s", status)
}

const (
	CondConfigSync = "Config Sync"
	CondBackend    = "Backend"
	CondNodeDriver = "Node Driver"
)

type ConditionTracker interface {
	Set(key string, value ConditionStatus, reason string)
	Clear(key string, reason ...string)
	List() []string
	LastModified() time.Time

	// Adds a listener that will be called whenever any condition is changed.
	// The listener will be called in a separate goroutine. Ensure that the
	// listener does not itself set or clear any conditions.
	AddListener(listener any)
}

func NewDefaultConditionTracker(logger *slog.Logger) ConditionTracker {
	ct := &defaultConditionTracker{
		logger:  logger,
		modTime: atomic.NewTime(time.Now()),
	}
	ct.conditions.Store(CondConfigSync, StatusPending)
	return ct
}

type defaultConditionTracker struct {
	conditions gsync.Map[string, ConditionStatus]
	logger     *slog.Logger
	modTime    *atomic.Time

	listenersMu sync.Mutex
	listeners   []func(key string, value *ConditionStatus, reason string)
}

func (ct *defaultConditionTracker) Set(key string, value ConditionStatus, reason string) {
	lg := ct.logger.With(
		"condition", key,
		"status", value,
		"reason", reason,
	)
	if v, ok := ct.conditions.Load(key); ok {
		if v != value {
			lg.Info("condition changed")
		}
	} else if v != value {
		lg.Info("condition set")
		ct.conditions.Store(key, value)
		ct.modTime.Store(time.Now())
		ct.notifyListeners(key, &value, reason)
	}
}

func (ct *defaultConditionTracker) Clear(key string, reason ...string) {
	lg := ct.logger.With(
		"condition", key,
	)
	if len(reason) > 0 {
		lg = lg.With("reason", reason[0])
	}
	if v, ok := ct.conditions.LoadAndDelete(key); ok {
		ct.modTime.Store(time.Now())
		lg.With(
			"previous", v,
		).Info("condition cleared")
		ct.notifyListeners(key, nil, "")
	}
}

func (ct *defaultConditionTracker) List() []string {
	var conditions []string
	ct.conditions.Range(func(key string, value ConditionStatus) bool {
		conditions = append(conditions, fmt.Sprintf("%s %s", key, value))
		return true
	})
	return conditions
}

func (ct *defaultConditionTracker) LastModified() time.Time {
	return ct.modTime.Load()
}

func (ct *defaultConditionTracker) AddListener(listener any) {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	switch listener := listener.(type) {
	case func():
		ct.listeners = append(ct.listeners, func(_ string, _ *ConditionStatus, _ string) {
			listener()
		})
	case func(string):
		ct.listeners = append(ct.listeners, func(key string, _ *ConditionStatus, _ string) {
			listener(key)
		})
	case func(string, *ConditionStatus):
		ct.listeners = append(ct.listeners, func(key string, value *ConditionStatus, _ string) {
			listener(key, value)
		})
	case func(string, *ConditionStatus, string):
		ct.listeners = append(ct.listeners, listener)
	default:
		panic(fmt.Sprintf("unsupported listener type %T", listener))
	}
}

func (ct *defaultConditionTracker) notifyListeners(key string, value *ConditionStatus, reason string) {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	for _, listener := range ct.listeners {
		go listener(key, value, reason)
	}
}
