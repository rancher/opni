package metrics

import (
	"fmt"
	"github.com/prometheus/common/model"
	"regexp"
	"text/template"
)

var KubeStates = []string{"Pending", "Running", "Succeeded", "Failed", "Unknown"}

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

func NewKubePodStateRule(
	podName string,
	namespace string,
	podState string,
	forDuration string,
	labels map[string]string,
	annotations map[string]string,
) (*AlertingRule, error) {
	if podName == "" {
		return nil, fmt.Errorf("aksjdhakjshdkij")
	}
	validPodState := false
	for _, state := range KubeStates {
		if state == podState {
			validPodState = true
		}
	}
	if !validPodState {
		return nil, fmt.Errorf("invalid pod state provided %s", podState)
	}
	dur, err := model.ParseDuration(forDuration)
	if err != nil {
		return nil, err
	}
	//handle empty namespace
	var namespaceFilter string
	if namespace != "" {
		namespaceFilter = "namespace=\"" + namespace + "\","
	} else {
		namespaceFilter = ""
	}
	return &AlertingRule{
		Alert:       "",
		Expr:        fmt.Sprintf("(%s{%s pod=\"%s\",state=\"%s\"} > bool 0)", KubePodStatusMetricName, namespaceFilter, podName, podState),
		For:         dur,
		Labels:      labels,
		Annotations: annotations,
	}, nil
}
