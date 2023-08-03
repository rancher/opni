/*
Shared definitions (constants & errors) for opni alerting
*/
package shared

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/lithammer/shortuuid"
	"github.com/rancher/opni/pkg/validation"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const SingleConfigId = "global"

const OpniAlertingCortexNamespace = "opni-alerting"

func NewAlertingRefId(prefixes ...string) string {
	return strings.Join(append(prefixes, shortuuid.New()), "-")
}

// config ids

const InternalSlackId = "slack"
const InternalEmailId = "email"
const InternalPagerdutyId = "pagerduty"
const InternalWebhookId = "webhook"
const InternalPushoverId = "pushover"
const InternalSNSId = "sns"
const InternalTelegramId = "telegram"
const InternalDiscordId = "discord"
const InternalOpsGenieId = "opsgenie"
const InternalVictorOpsId = "victorops"
const InternalWechatId = "wechat"

type OpniReceiverId struct {
	Namespace  string
	ReceiverId string
}

func NewOpniReceiverName(id OpniReceiverId) string {
	return strings.Join([]string{"opni", id.Namespace, id.ReceiverId}, "__")
}

func ExtractReceiverId(receiverName string) (*OpniReceiverId, error) {
	parts := strings.Split(receiverName, "__")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid receiver name format: %s", receiverName)
	}
	return &OpniReceiverId{
		Namespace:  parts[1],
		ReceiverId: parts[2],
	}, nil
}

// Condition constants
var ComparisonOperators = []string{"<", ">", "<=", ">=", "=", "!="}
var KubeStates = []string{"Pending", "Running", "Succeeded", "Failed", "Unknown"}

const CortexDistributor = "distributor"
const CortexIngester = "ingester"
const CortexRuler = "ruler"
const CortexPurger = "purger"
const CortexCompactor = "compactor"
const CortexStoreGateway = "store-gateway"
const CortexQueryFrontend = "query-frontend"
const CortexQuerier = "querier"

var CortexComponents = []string{
	CortexDistributor,
	CortexIngester,
	CortexRuler,
	CortexPurger,
	CortexCompactor,
	CortexStoreGateway,
	CortexQueryFrontend,
	CortexQuerier,
}

// Storage types

const AgentDisconnectStorageType = "agent-disconnect"

// Datasources & Versioning

// labels
const OpniTitleLabel = "OpniTitleLabel"
const OpniBodyLabel = "OpniBodyLabel"
const BackendConditionNameLabel = "opniname"
const BackendConditionClusterIdLabel = "clusterId"
const BackendConditionSeverityLabel = "severity"

// Operator container & service definitions

const (
	ConfigMountPath          = "/etc/config"
	DataMountPath            = "/var/lib"
	AlertManagerConfigKey    = "alertmanager.yaml"
	InternalRoutingConfigKey = "internal-routing.yaml"
	AlertmanagerService      = "opni-alertmanager-alerting"
	EmitterService           = "opni-emitter-alerting"
	AlertingHookReceiverName = "opni.default.hook"
	AlertingDefaultHookName  = "/opni/hook"
	AlertingDefaultHookPort  = 3000
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
		CortexHandlerURL:  managementUrl + AlertingDefaultHookName,
	})
	if err != nil {
		return b, err
	}
	return b, nil
}

// Error declarations

var (
	AlertingErrNotImplemented      = WithUnimplementedError("Not implemented")
	AlertingErrInvalidSlackChannel = validation.Error("Slack channel invalid : must start with '#'")
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
