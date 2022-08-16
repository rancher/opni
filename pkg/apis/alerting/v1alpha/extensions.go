package v1alpha

import "github.com/rancher/opni/pkg/alerting/shared"

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

	EnumConditionToImplementation[AlertType_SYSTEM] = AlertTypeDetails{
		Type: &AlertTypeDetails_System{},
	}
	EnumConditionToImplementation[AlertType_KUBE_STATE] = AlertTypeDetails{
		Type: &AlertTypeDetails_KubeState{},
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
		if v.GetType() == a.GetType() {
			return nil
		}
	}
	return shared.AlertingErrNotImplemented
}
