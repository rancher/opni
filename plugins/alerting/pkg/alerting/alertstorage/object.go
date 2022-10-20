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
