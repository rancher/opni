package shared

import "fmt"

type AlertingOptions struct {
	Endpoints         []string
	ConfigMap         string
	Namespace         string
	StatefulSet       string
	CortexHookHandler string
}

type NewAlertingOptions struct {
	Namespace             string
	WorkerNodesService    string
	WorkerNodePort        int
	WorkerStatefulSet     string
	ControllerNodeService string
	ControllerNodePort    int
	ControllerClusterPort int
	ControllerStatefulSet string
	ConfigMap             string
	ManagementHookHandler string
}

func (a *NewAlertingOptions) GetControllerEndpoint() string {
	return fmt.Sprintf("%s:%d", a.ControllerNodeService, a.ControllerNodePort)
}

func (a *NewAlertingOptions) GetWorkerEndpoint() string {
	return fmt.Sprintf("%s:%d", a.WorkerNodesService, a.WorkerNodePort)
}
