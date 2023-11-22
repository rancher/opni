package v1

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/ttacon/chalk"
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
	GetLogLevel() slog.Level
}

var _ TimestampedLog = (*LogEntry)(nil)
var _ TimestampedLog = (*StateTransition)(nil)

func (s *LogEntry) GetLogLevel() slog.Level {
	return slog.Level(s.Level)
}

func (s *StateTransition) GetLogLevel() slog.Level {
	return slog.LevelInfo
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

// Set sets the revision to the given value, and clears the timestamp.
func (r *Revision) Set(revision int64) {
	if r == nil {
		panic("revision is nil")
	}
	if r.Revision == nil {
		r.Revision = &revision
	} else {
		*r.Revision = revision
		r.Timestamp = nil
	}
}

func VerbGet() *PermissionVerb {
	return &PermissionVerb{
		Verb: "GET",
	}
}

func (v *PermissionVerb) InList(in []*PermissionVerb) bool {
	for _, verb := range in {
		if v.GetVerb() == verb.GetVerb() {
			return true
		}
	}
	return false
}

func (r *Role) GetRevision() int64 {
	rev := r.GetMetadata().GetResourceVersion()
	if rev == "" {
		return 0
	}
	revision, err := strconv.ParseInt(rev, 10, 64)
	if err != nil {
		panic(err)
	}
	return revision
}
