package agent

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

func (s ConditionStatus) String() string {
	switch s {
	case StatusPending:
		return "Pending"
	case StatusFailure:
		return "Failure"
	}
	return ""
}

const (
	CondRemoteWrite = "Remote Write"
	CondRuleSync    = "Rule Sync"
	CondConfigSync  = "Config Sync"
)

type ConditionTracker interface {
	Set(key string, value ConditionStatus, reason string)
	Clear(key string, reason ...string)
	List() []string
}

func NewConditionTracker(logger *zap.SugaredLogger) ConditionTracker {
	ct := &conditionTracker{
		logger: logger,
	}
	// ct.conditions.Store(CondRemoteWrite, StatusPending)
	// ct.conditions.Store(CondRuleSync, StatusPending)
	ct.conditions.Store(CondConfigSync, StatusPending)
	return ct
}

type conditionTracker struct {
	conditions gsync.Map[string, ConditionStatus]
	logger     *zap.SugaredLogger
}

func (ct *conditionTracker) Set(key string, value ConditionStatus, reason string) {
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

func (ct *conditionTracker) Clear(key string, reason ...string) {
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

func (ct *conditionTracker) List() []string {
	var conditions []string
	ct.conditions.Range(func(key string, value ConditionStatus) bool {
		conditions = append(conditions, fmt.Sprintf("%s %s", key, value))
		return true
	})
	return conditions
}
