package metrics

import (
	"fmt"
	"github.com/prometheus/common/model"
)

var KubePodStates = [...]string{"Pending", "Running", "Succeeded", "Failed", "Unknown"}

const KubePodStatusMetricName = "kube_pod_status_phase"

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
	for _, state := range KubePodStates {
		if state == podState {
			validPodState = true
		}
	}
	if !validPodState {
		return nil, fmt.Errorf("alkjsdhkjahsdkjahsd")
	}
	dur, err := model.ParseDuration(forDuration)
	if err != nil {
		return nil, err
	}
	//handle empty namespace
	var namespaceFilter string
	if namespace != "" {
		namespaceFilter = "{namespace=\"" + namespace + "\"},"
	} else {
		namespaceFilter = ""
	}
	return &AlertingRule{
		Alert:       "",
		Expr:        fmt.Sprintf("(%s{%s pod=\"%s\",state=\"%s\"} > 0)", KubePodStatusMetricName, namespaceFilter, podName, podState),
		For:         dur,
		Labels:      labels,
		Annotations: annotations,
	}, nil
}
