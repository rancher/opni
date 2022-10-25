package health

import (
	"fmt"
	"sync"
	"time"

	gsync "github.com/kralicky/gpkg/sync"
	"go.uber.org/atomic"
	"go.uber.org/zap"
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

const (
	CondConfigSync = "Config Sync"
	CondBackend    = "Backend"
)

type ConditionTracker interface {
	Set(key string, value ConditionStatus, reason string)
	Clear(key string, reason ...string)
	List() []string
	LastModified() time.Time

	// Adds a listener that will be called whenever any condition is changed.
	// The listener will be called in a separate goroutine. Ensure that the
	// listener does not itself set or clear any conditions.
	AddListener(listener func())
}

func NewDefaultConditionTracker(logger *zap.SugaredLogger) ConditionTracker {
	ct := &defaultConditionTracker{
		logger:  logger,
		modTime: atomic.NewTime(time.Now()),
	}
	ct.conditions.Store(CondConfigSync, StatusPending)
	return ct
}

type defaultConditionTracker struct {
	conditions gsync.Map[string, ConditionStatus]
	logger     *zap.SugaredLogger
	modTime    *atomic.Time

	listenersMu sync.Mutex
	listeners   []func()
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
		ct.notifyListeners()
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
		ct.notifyListeners()
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

func (ct *defaultConditionTracker) AddListener(listener func()) {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	ct.listeners = append(ct.listeners, listener)
}

func (ct *defaultConditionTracker) notifyListeners() {
	ct.listenersMu.Lock()
	defer ct.listenersMu.Unlock()
	for _, listener := range ct.listeners {
		go listener()
	}
}
