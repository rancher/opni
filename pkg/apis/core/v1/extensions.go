package v1

import (
	"fmt"
	"strings"
	"time"

	"github.com/ttacon/chalk"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type CorePermissionType string

const (
	PermissionTypeCluster   CorePermissionType = "cluster"
	PermissionTypeNamespace CorePermissionType = "namespace"
	PermissionTypeAPI       CorePermissionType = "api"
)

func (s *HealthStatus) Summary() string {
	if s.Status == nil || s.Health == nil {
		return "Unknown"
	}
	if !s.Status.Connected {
		return "Disconnected"
	}
	if len(s.Health.Conditions) > 0 {
		return fmt.Sprintf("Unhealthy: %s", strings.Join(s.Health.Conditions, ", "))
	}
	if !s.Health.Ready {
		return fmt.Sprintf("Not ready (unknown reason)")
	}
	return "Healthy"
}

type TimestampedLog interface {
	GetTimestamp() *timestamppb.Timestamp
	GetMsg() string
	GetLogLevel() zapcore.Level
}

var _ TimestampedLog = (*LogEntry)(nil)
var _ TimestampedLog = (*StateTransition)(nil)

func (s *LogEntry) GetLogLevel() zapcore.Level {
	return zapcore.Level(s.Level)
}

func (s *StateTransition) GetLogLevel() zapcore.Level {
	return zapcore.InfoLevel
}

func (s *StateTransition) GetMsg() string {
	return fmt.Sprintf("State changed to %s",
		stateColor(s.State).Color(s.State.String()))
}

func stateColor(state TaskState) chalk.Color {
	switch state {
	case TaskState_Pending:
		return chalk.Yellow
	case TaskState_Running:
		return chalk.Blue
	case TaskState_Failed, TaskState_Canceled:
		return chalk.Red
	case TaskState_Completed:
		return chalk.Green
	default:
		return chalk.White
	}
}

func NewRevision(revision int64, maybeTimestamp ...time.Time) *Revision {
	return &Revision{
		Revision: &revision,
		Timestamp: func() *timestamppb.Timestamp {
			if len(maybeTimestamp) > 0 && !maybeTimestamp[0].IsZero() {
				return timestamppb.New(maybeTimestamp[0])
			}
			return nil
		}(),
	}
}

func (r *Revision) Set(revision int64) {
	if r == nil {
		panic("revision is nil")
	}
	if r.Revision == nil {
		r.Revision = &revision
	} else {
		*r.Revision = revision
	}
}
