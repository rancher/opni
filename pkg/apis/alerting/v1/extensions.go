package v1

import (
	"reflect"

	"github.com/rancher/opni/pkg/alerting/shared"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
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
	EnumConditionToDatasource[AlertType_DOWNSTREAM_CAPABILTIY] = &system
	EnumConditionToDatasource[AlertType_KUBE_STATE] = &monitoring
	EnumConditionToDatasource[AlertType_CPU_SATURATION] = &monitoring
	EnumConditionToDatasource[AlertType_MEMORY_SATURATION] = &monitoring
	EnumConditionToDatasource[AlertType_FS_SATURATION] = &monitoring
	EnumConditionToDatasource[AlertType_COMPOSITION] = &monitoring
	EnumConditionToDatasource[AlertType_CONTROL_FLOW] = &monitoring
	EnumConditionToDatasource[AlertType_PROMETHEUS_QUERY] = &monitoring

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
	EnumConditionToImplementation[AlertType_FS_SATURATION] = AlertTypeDetails{
		Type: &AlertTypeDetails_Fs{
			Fs: &AlertConditionFilesystemSaturation{},
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

	EnumConditionToImplementation[AlertType_PROMETHEUS_QUERY] = AlertTypeDetails{
		Type: &AlertTypeDetails_PrometheusQuery{
			PrometheusQuery: &AlertConditionPrometheusQuery{},
		},
	}
	EnumConditionToImplementation[AlertType_DOWNSTREAM_CAPABILTIY] = AlertTypeDetails{
		Type: &AlertTypeDetails_DownstreamCapability{
			DownstreamCapability: &AlertConditionDownstreamCapability{},
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

// stop-gap solution, until we move to the new versin of the API
func (a *AlertCondition) GetClusterId() *corev1.Reference {
	if a.GetAlertType().GetSystem() != nil {
		return a.GetAlertType().GetSystem().GetClusterId()
	}
	if a.GetAlertType().GetPrometheusQuery() != nil {
		return a.GetAlertType().GetPrometheusQuery().GetClusterId()
	}
	if a.GetAlertType().GetKubeState() != nil {
		return &corev1.Reference{Id: a.GetAlertType().GetKubeState().ClusterId}
	}
	if a.GetAlertType().GetCpu() != nil {
		return a.GetAlertType().GetCpu().GetClusterId()
	}
	if a.GetAlertType().GetMemory() != nil {
		return a.GetAlertType().GetMemory().GetClusterId()
	}
	if a.GetAlertType().GetFs() != nil {
		return a.GetAlertType().GetFs().GetClusterId()
	}
	return nil
}

// stop-gap solution until we move to the new version of the API
func (a *AlertCondition) SetClusterId(clusterId *corev1.Reference) error {
	if a.GetAlertType().GetSystem() != nil {
		a.GetAlertType().GetSystem().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetPrometheusQuery() != nil {
		a.GetAlertType().GetPrometheusQuery().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetKubeState() != nil {
		a.GetAlertType().GetKubeState().ClusterId = clusterId.Id
		return nil
	}
	if a.GetAlertType().GetCpu() != nil {
		a.GetAlertType().GetCpu().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetMemory() != nil {
		a.GetAlertType().GetMemory().ClusterId = clusterId
		return nil
	}
	if a.GetAlertType().GetFs() != nil {
		a.GetAlertType().GetFs().ClusterId = clusterId
		return nil
	}
	return shared.WithInternalServerErrorf("AlertCondition could not find its clusterId")
}
