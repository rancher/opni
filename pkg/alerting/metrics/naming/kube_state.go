package naming

import (
	"regexp"
	"text/template"
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
