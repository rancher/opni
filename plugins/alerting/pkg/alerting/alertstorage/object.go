package alertstorage

import "github.com/rancher/opni/pkg/health"

type AgentIncident struct {
	ConditionId string               `json:"conditionId"`
	Steps       []*AgentIncidentStep `json:"steps"`
}

type AgentIncidentStep struct {
	health.StatusUpdate `json:",inline"`
	AlertFiring         bool `json:"alertFiring"`
}

func (a AgentIncident) isEquivalentState(other AgentIncidentStep) bool {
	if len(a.Steps) == 0 {
		return false
	}
	cur := a.Steps[len(a.Steps)-1]
	if cur.StatusUpdate.Status.Connected == other.StatusUpdate.Status.Connected && cur.AlertFiring == other.AlertFiring {
		return true
	}
	// different enough that we should add a new step
	return false
}
