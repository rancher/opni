package metrics

import (
	"bytes"
	"fmt"
	"regexp"
	"text/template"

	"github.com/prometheus/common/model"
	"github.com/rancher/opni/pkg/alerting/shared"
)

// KubeObjMetricNameMatcher
//
// PromQl Matcher for kube state metrics
const KubeObjMetricNameMatcher = "kube_.*_status_phase"

// KubeObjTypeExtractor
//
// Group 1 extracts the object type
var KubeObjTypeExtractor = regexp.MustCompile("kube_(.*)_status_phase")
var KubeObjMetricCreator = template.Must(template.New("KubeObject").Parse("kube_{{.ObjType}}_status_phase"))

const KubePodStatusMetricName = "kube_pod_status_phase"
const KubeMetricsIsDefinedMetricName = "kube_namespace_created"

var KubeStateAlertIdLabels = []string{}
var KubeStateAnnotations = map[string]string{}

func NewKubeStateRule(
	objType string,
	objName string,
	namespace string,
	podState string,
	forDuration string,
	annotations map[string]string,
) (*AlertingRule, error) {
	if objType == "" {
		return nil, fmt.Errorf("kubernetes object type should not be empty")
	}
	if objName == "" {
		return nil, fmt.Errorf("kubernetes objects cannot have an empty name")
	}
	var kubeMetricNameBuffer bytes.Buffer
	err := KubeObjMetricCreator.Execute(&kubeMetricNameBuffer, map[string]string{
		"ObjType": objType,
	})
	if err != nil {
		return nil, err
	}
	kubeMetricName := kubeMetricNameBuffer.String()
	objectFilter := objType + fmt.Sprintf("= \"%s\"", objName)
	validState := false
	for _, state := range shared.KubeStates {
		if state == podState {
			validState = true
		}
	}
	if !validState {
		return nil, fmt.Errorf("invalid pod state provided %s", podState)
	}
	dur, err := model.ParseDuration(forDuration)
	if err != nil {
		return nil, err
	}
	//handle empty namespace
	var namespaceFilter string
	if namespace != "" {
		namespaceFilter = "namespace=\"" + namespace + "\""
	} else {
		namespaceFilter = ""
	}
	return &AlertingRule{
		Alert:       "",
		Expr:        fmt.Sprintf("(%s{%s, %s, state=\"%s\"} > bool 0)", kubeMetricName, namespaceFilter, objectFilter, podState),
		For:         dur,
		Labels:      annotations,
		Annotations: annotations,
	}, nil
}
