package health

import (
	"fmt"

	gsync "github.com/kralicky/gpkg/sync"
	"go.uber.org/zap"
)

type ConditionStatus int32

const (
	StatusPending ConditionStatus = 0
	StatusFailure ConditionStatus = 1
)

const (
	CondConfigSync = "Config Sync"
)

func (s ConditionStatus) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusFailure:
		return "Failure"
	}
	return ""
}

type ConditionTracker interface {
	Set(key string, value ConditionStatus, reason string)
	Clear(key string, reason ...string)
	List() []string
}

func NewDefaultConditionTracker(logger *zap.SugaredLogger) ConditionTracker {
	ct := &defaultConditionTracker{
		logger: logger,
	}
	ct.conditions.Store(CondConfigSync, StatusPending)
	return ct
}

type defaultConditionTracker struct {
	conditions gsync.Map[string, ConditionStatus]
	logger     *zap.SugaredLogger
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
	} else {
		lg.Info("condition set")
	}
	ct.conditions.Store(key, value)
}

func (ct *defaultConditionTracker) Clear(key string, reason ...string) {
	lg := ct.logger.With(
		"condition", key,
	)
	if len(reason) > 0 {
		lg = lg.With("reason", reason[0])
	}
	if v, ok := ct.conditions.LoadAndDelete(key); ok {
		lg.With(
			"previous", v,
		).Info("condition cleared")
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
