package cortex_test

/**
Functions to help proxy a cortex alert paylaod
**/

import (
	"fmt"

	"github.com/prometheus/alertmanager/notify/webhook"
	"github.com/prometheus/alertmanager/template"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	corev1 "github.com/rancher/opni/pkg/apis/core/v1"
	"github.com/tidwall/gjson"
)

type MockCortexPayload struct {
	webhook.Message
}

func NewSimpleMockAlertManagerPayloadFromAnnotations(ann map[string]string) *MockCortexPayload {
	kv := template.KV(ann)
	return &MockCortexPayload{
		Message: webhook.Message{
			Data: &template.Data{
				Alerts: []template.Alert{
					{
						Annotations: kv,
					},
				},
			},
		},
	}
}

var RequiredCortexWebhookAnnotationIdentifiers = []string{"conditionId"}

var OpniDisconnect *alertingv1.AlertCondition = &alertingv1.AlertCondition{
	Name:        "Disconnected Opni Agent {{ .agentId }} ",
	Description: "Opni agent {{ .agentId }} has been disconnected for more than {{ .timeout }}",
	Labels:      []string{"opni", "agent", "system"},
	Severity:    alertingv1.OpniSeverity_Critical,
	AlertType:   &alertingv1.AlertTypeDetails{Type: &alertingv1.AlertTypeDetails_System{}},
}

func ParseCortexPayloadBytes(inputPayload []byte) ([]gjson.Result, error) {
	strPayload := string(inputPayload)
	if !gjson.Valid(strPayload) {
		return []gjson.Result{}, fmt.Errorf(
			fmt.Sprintf("failed to parse request body to json %s", strPayload))

	}
	// parse alerts path
	// should be in the form
	// "alerts": [
	//    {
	//      "status": "<resolved|firing>",
	//      "labels": <object>,
	//      "annotations": <object>,
	//      "startsAt": "<rfc3339>",
	//      "endsAt": "<rfc3339>",
	//      "generatorURL": <string>,      // identifies the entity that caused the alert
	//      "fingerprint": <string>        // fingerprint to identify the alert
	//    },
	alertArr := gjson.Get(strPayload, "alerts.#.annotations")
	if !alertArr.Exists() {
		return []gjson.Result{}, fmt.Errorf("no alerts or alert annotations found in payload")
	}
	return alertArr.Array(), nil
}

func ParseAlertManagerWebhookPayload(annotations []gjson.Result) ([]*alertingv1.TriggerAlertsRequest, []error) {
	var errors []error
	var opniRequests []*alertingv1.TriggerAlertsRequest
	for _, annotation := range annotations {
		resAnnotations := make(map[string]string)
		result := annotation.Map()
		// if map empty, something went wrong
		if len(result) == 0 {
			errors = append(errors, fmt.Errorf("could not parse annotation %s", annotation.String()))
			opniRequests = append(opniRequests, nil)
			continue
		}
		anyFailed := false
		res := &alertingv1.TriggerAlertsRequest{}
	IDENTIFIERS:
		for _, identifier := range RequiredCortexWebhookAnnotationIdentifiers {
			if _, ok := result[identifier]; !ok {
				errors = append(errors, fmt.Errorf(
					"cortex Annotation missing required opni identifier to pass on alert%s", annotation.String()))
				opniRequests = append(opniRequests, nil)
				anyFailed = true
				break
			} else {
				switch identifier {
				case "conditionId":
					res.ConditionId = &corev1.Reference{Id: result[identifier].String()}
				default:
					errors = append(errors, fmt.Errorf("unhandled opni identifier %s", identifier))
					opniRequests = append(opniRequests, nil)
					anyFailed = true
					break IDENTIFIERS
				}
			}
			delete(result, identifier)
		}
		for key := range result {
			resAnnotations[key] = result[key].String()
		}
		res.Annotations = resAnnotations
		if anyFailed {
			continue
		} else {
			errors = append(errors, nil)
			opniRequests = append(opniRequests, res)
		}
	}
	return opniRequests, errors
}
