package common

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
