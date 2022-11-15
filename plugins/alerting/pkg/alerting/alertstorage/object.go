package alertstorage

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rancher/opni/pkg/alerting/shared"
	"github.com/rancher/opni/pkg/health"
)

func HandleInternalIncidentType(systemConditionType string) InternalIncident {
	switch systemConditionType {
	case shared.AgentDisconnectStorageType:
		return &AgentIncident{}
	default:
		panic(fmt.Sprintf("no such internal incident type string  %s", systemConditionType))
	}
}

type AgentIncident struct {
	ConditionId string                 `json:"conditionId"`
	Steps       []InternalIncidentStep `json:"steps"`
}

func (a *AgentIncident) New(conditionId string, steps ...InternalIncidentStep) {
	a.ConditionId = conditionId
	a.Steps = steps
}

func (a *AgentIncident) AddStep(step InternalIncidentStep) {
	a.Steps = append(a.Steps, step)
}

func (a *AgentIncident) GetConditionId() string {
	return a.ConditionId
}

func (a *AgentIncident) isEquivalentState(other InternalIncidentStep) bool {
	if len(a.Steps) == 0 {
		return false
	}
	cur := a.Steps[len(a.Steps)-1]
	if (cur.IsHealthy() == other.IsHealthy()) && (cur.IsFiring() == other.IsFiring()) {
		return true
	}
	// different enough that we should add a new step
	return false
}

// we do "type casting" to AgentIncidentStep here
func (a *AgentIncident) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ConditionId string                 `json:"conditionId"`
		Steps       []InternalIncidentStep `json:"steps"`
	}{
		ConditionId: a.ConditionId,
		Steps:       a.Steps,
	})
}

// we do "type casting" to AgentIncidentStep here
func (a *AgentIncident) UnmarshalJSON(data []byte) error {
	aux := struct {
		ConditionId string               `json:"conditionId"`
		Steps       []*AgentIncidentStep `json:"steps"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	a.ConditionId = aux.ConditionId
	a.Steps = make([]InternalIncidentStep, len(aux.Steps))
	for i, step := range aux.Steps {
		a.Steps[i] = step
	}
	return nil
}

func (a *AgentIncident) GetSteps() []InternalIncidentStep {
	steps := make([]InternalIncidentStep, len(a.Steps))
	copy(steps, a.Steps)
	return steps
}

type AgentIncidentStep struct {
	health.StatusUpdate `json:",inline"`
	AlertFiring         bool `json:"alertFiring"`
}

var _ InternalIncidentStep = &AgentIncidentStep{}

func (a *AgentIncidentStep) IsFiring() bool {
	return a.AlertFiring
}

func (a *AgentIncidentStep) IsHealthy() bool {
	return a.StatusUpdate.Status.Connected
}

func (a *AgentIncidentStep) GetTimestamp() *timestamppb.Timestamp {
	return a.StatusUpdate.Status.Timestamp
}

func (a *AgentIncidentStep) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		health.StatusUpdate `json:",inline"`
		AlertFiring         bool `json:"alertFiring"`
	}{
		StatusUpdate: a.StatusUpdate,
		AlertFiring:  a.AlertFiring,
	})
}

func (a *AgentIncidentStep) UnmarshalJSON(data []byte) error {
	type Alias AgentIncidentStep
	aux := struct {
		*Alias
	}{
		Alias: (*Alias)(a),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	return nil
}

var _ InternalIncident = &AgentIncident{}
