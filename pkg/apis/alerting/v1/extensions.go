package v1

import (
	"reflect"

	"github.com/rancher/opni/pkg/alerting/shared"
)

// EnumConditionToDatasource
//
// nil values indicate they are composition alerts
// we need this since each alert tree must only have one datasource atm
var EnumConditionToDatasource = make(map[AlertType]*string)

var EnumConditionToImplementation = make(map[AlertType]AlertTypeDetails)

func init() {
	//logging := shared.LoggingDatasource
	monitoring := shared.MonitoringDatasource
	system := shared.SystemDatasource
	EnumConditionToDatasource[AlertType_SYSTEM] = &system
	EnumConditionToDatasource[AlertType_KUBE_STATE] = &monitoring
	EnumConditionToDatasource[AlertType_CPU_SATURATION] = &monitoring
	EnumConditionToDatasource[AlertType_MEMORY_SATURATION] = &monitoring
	EnumConditionToDatasource[AlertType_FS_SATURATION] = &monitoring
	EnumConditionToDatasource[AlertType_COMPOSITION] = &monitoring
	EnumConditionToDatasource[AlertType_CONTROL_FLOW] = &monitoring

	EnumConditionToImplementation[AlertType_SYSTEM] = AlertTypeDetails{
		Type: &AlertTypeDetails_System{
			System: &AlertConditionSystem{},
		},
	}
	EnumConditionToImplementation[AlertType_KUBE_STATE] = AlertTypeDetails{
		Type: &AlertTypeDetails_KubeState{
			KubeState: &AlertConditionKubeState{},
		},
	}

	EnumConditionToImplementation[AlertType_CPU_SATURATION] = AlertTypeDetails{
		Type: &AlertTypeDetails_Cpu{
			Cpu: &AlertConditionCPUSaturation{},
		},
	}
	EnumConditionToImplementation[AlertType_MEMORY_SATURATION] = AlertTypeDetails{
		Type: &AlertTypeDetails_Memory{
			Memory: &AlertConditionMemorySaturation{},
		},
	}
	EnumConditionToImplementation[AlertType_FS_SATURATION] = AlertTypeDetails{}
	EnumConditionToImplementation[AlertType_COMPOSITION] = AlertTypeDetails{
		Type: &AlertTypeDetails_Composition{
			Composition: &AlertConditionComposition{},
		},
	}
	EnumConditionToImplementation[AlertType_CONTROL_FLOW] = AlertTypeDetails{
		Type: &AlertTypeDetails_ControlFlow{
			ControlFlow: &AlertConditionControlFlow{},
		},
	}
}

func EnumHasImplementation(a AlertType) error {
	if _, ok := EnumConditionToImplementation[a]; !ok {
		return shared.AlertingErrNotImplemented
	}
	return nil
}

func DetailsHasImplementation(a *AlertTypeDetails) error {
	for _, v := range EnumConditionToImplementation {
		if reflect.TypeOf(v.GetType()) == reflect.TypeOf(a.GetType()) {
			return nil
		}
	}
	return shared.AlertingErrNotImplemented
}

type Templateable interface {
	ListTemplates() []string
}

var _ Templateable = &AlertTypeDetails{}

func (a *AlertTypeDetails) ListTemplates() []string {
	if a.GetSystem() != nil {
		return a.GetSystem().ListTemplates()
	}
	if a.GetKubeState() != nil {
		return a.GetKubeState().ListTemplates()
	}
	if a.GetComposition() != nil {
		return a.GetComposition().ListTemplates()
	}
	if a.GetControlFlow() != nil {
		return a.GetControlFlow().ListTemplates()
	}
	panic("No templates returned for alert type")
}
func (a *AlertConditionSystem) ListTemplates() []string {
	return []string{
		"agentId",
		"timeout",
	}
}

func (a *AlertConditionKubeState) ListTemplates() []string {
	return []string{}
}

func (a *AlertConditionComposition) ListTemplates() []string {
	return []string{}
}

func (a *AlertConditionControlFlow) ListTemplates() []string {
	return []string{}
}

func (r *RoutingRelationships) InvolvedConditionsForEndpoint(endpointId string) []string {
	res := []string{}
	for conditionId, endpointMap := range r.GetConditions() {
		for endpoint := range endpointMap.GetEndpoints() {
			if endpoint == endpointId {
				res = append(res, conditionId)
				break
			}
		}
	}
	return res
}

func ShouldCreateRoutingNode(new, old *AttachedEndpoints) bool {
	// only create if we go from having no endpoints to having some
	if new == nil || len(new.Items) == 0 {
		return false
	} else if (old == nil || len(old.Items) == 0) && new != nil && len(new.Items) > 0 {
		return true
	}
	return false // should update pre-existing routing-node
}

func ShouldUpdateRoutingNode(new, old *AttachedEndpoints) bool {
	// only update if both are specified
	if new != nil && len(new.Items) > 0 && old != nil && len(old.Items) > 0 {
		return true
	}
	return false
}

func ShouldDeleteRoutingNode(new, old *AttachedEndpoints) bool {
	// only delete if we go from having endpoints to having none
	if (new == nil || len(new.Items) > 0) && old != nil && len(old.Items) > 0 {
		return true
	}
	return false
}
