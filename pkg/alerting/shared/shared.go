/*
Shared definitions (constants & errors) for opni alerting
*/
package shared

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Condition constants
var ComparisonOperators = []string{"<", ">", "<=", ">=", "=", "!="}
var KubeStates = []string{"Pending", "Running", "Succeeded", "Failed", "Unknown"}

// Datasources & Versioning

const LocalBackendEnvToggle = "OPNI_ALERTING_BACKEND_LOCAL"
const LocalAlertManagerPath = "/tmp/alertmanager.yaml"
const BackendConditionIdLabel = "conditionId"

const (
	AlertingV1Alpha      = "v1alpha"
	MonitoringDatasource = "monitoring"
	LoggingDatasource    = "logging"
	SystemDatasource     = "system"
)

// Operator container & service definitions

const (
	ConfigMountPath                        = "/etc/config"
	DataMountPath                          = "/var/lib/alertmanager/data"
	AlertManagerConfigKey                  = "alertmanager.yaml"
	InternalRoutingConfigKey               = "internal-routing.yaml"
	OperatorAlertingControllerServiceName  = "opni-alerting-controller"
	OperatorAlertingClusterNodeServiceName = "opni-alerting"
	AlertingHookReceiverName               = "opni.hook"
	AlertingCortexHookHandler              = "/management/alerting/cortexHandler"
)

var (
	PublicLabels        = map[string]string{}
	PublicServiceLabels = map[string]string{}
)

func LabelWithAlert(label map[string]string) map[string]string {
	label["app.kubernetes.io/name"] = "opni-alerting"
	return label
}

// Default Operator AM config

const route = `
route:
  group_by: ['alertname']
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 1h
  receiver : '{{ .CortexHandlerName }}'
`

/*
Original:
receivers:
  - name: 'web.hook'
    webhook_configs:
  - url: 'http://127.0.0.1:5001/'
*/
const receivers = `
receivers:
  - name: '{{ .CortexHandlerName }}'
    webhook_configs:
      - url: '{{ .CortexHandlerURL }}'
`

const inihibit_rules = `
inhibit_rules:
- source_match:
    severity: 'critical'
  target_match:
    severity: 'warning'
  equal: ['alertname', 'dev', 'instance']`

var DefaultAlertManager = template.Must(template.New("DefaultManagementHook").Parse(strings.Join([]string{
	strings.TrimSpace(route),
	receivers,
	strings.TrimSpace(inihibit_rules),
}, "\n")))

type DefaultAlertManagerInfo struct {
	CortexHandlerName string
	CortexHandlerURL  string
}

func DefaultAlertManagerConfig(managementUrl string) (bytes.Buffer, error) {
	templateToFill := DefaultAlertManager
	var b bytes.Buffer
	err := templateToFill.Execute(&b, DefaultAlertManagerInfo{
		CortexHandlerName: AlertingHookReceiverName,
		CortexHandlerURL:  managementUrl + AlertingCortexHookHandler,
	})
	if err != nil {
		return b, err
	}
	return b, nil
}

func BackendDefaultFile(managementUrl string) error {
	b, err := DefaultAlertManagerConfig(managementUrl)
	if err != nil {
		return err
	}
	err = os.WriteFile(LocalAlertManagerPath, b.Bytes(), 0644)
	return err
}

// Error declarations

var (
	AlertingErrNotImplemented           = WithUnimplementedError("Not implemented")
	AlertingErrNotImplementedNOOP       = WithUnimplementedError("Alerting NOOP : Not implemented")
	AlertingErrParseBucket              = WithInternalServerError("Failed to parse bucket index")
	AlertingErrBucketIndexInvalid       = WithInternalServerError("Bucket index is invalid")
	AlertingErrInvalidSlackChannel      = validation.Error("Slack channel invalid : must start with '#'")
	AlertingErrK8sRuntime               = WithInternalServerError("K8s Runtime error")
	AlertingErrMismatchedImplementation = validation.Error("Alerting endpoint did not match the given implementation")
)

type UnimplementedError struct {
	message string
}

type InternalServerError struct {
	message string
}

type NotFoundError struct {
	message string
}

func (e *NotFoundError) Error() string {
	return e.message
}

func (e *NotFoundError) GRPCStatus() *status.Status {
	return status.New(codes.NotFound, e.message)
}

func WithNotFoundError(msg string) error {
	return &NotFoundError{
		message: msg,
	}
}

func WithNotFoundErrorf(format string, args ...interface{}) error {
	return &NotFoundError{
		message: fmt.Errorf(format, args...).Error(),
	}
}

func (e *InternalServerError) Error() string {
	return e.message
}

func (e *InternalServerError) GRPCStatus() *status.Status {
	return status.New(codes.Internal, e.message)
}

func WithInternalServerError(msg string) error {
	return &InternalServerError{
		message: msg,
	}
}

func WithInternalServerErrorf(format string, args ...interface{}) error {
	return &InternalServerError{
		message: fmt.Errorf(format, args...).Error(),
	}
}

func (e *UnimplementedError) Error() string {
	return e.message
}

func (e *UnimplementedError) GRPCStatus() *status.Status {
	return status.New(codes.Unimplemented, e.message)
}

func WithUnimplementedError(msg string) error {
	return &UnimplementedError{
		message: msg,
	}
}

func WithUnimplementedErrorf(format string, args ...interface{}) error {
	return &UnimplementedError{
		message: fmt.Errorf(format, args...).Error(),
	}
}
