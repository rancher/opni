package alertstorage

import "google.golang.org/protobuf/types/known/timestamppb"

type InternalIncidentStep interface {
	IsFiring() bool
	IsHealthy() bool
	GetTimestamp() *timestamppb.Timestamp
	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}

type InternalIncident interface {
	GetConditionId() string
	isEquivalentState(other InternalIncidentStep) bool
	GetSteps() []InternalIncidentStep
	AddStep(step InternalIncidentStep)
	New(conditionId string, steps ...InternalIncidentStep)
	MarshalJSON() ([]byte, error)
	UnmarshalJSON([]byte) error
}
