package metrics

import (
	"bytes"
	"fmt"
	"regexp"
	"text/template"

	"github.com/prometheus/common/model"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/utils/strings/slices"
)

const MemoryDeviceFilter = "device"
const MemoryMatcherName = "node_memory_.*_bytes"

var MemoryUsageTypeExtractor = regexp.MustCompile("node_memory_(.*)_bytes")
var MemoryMatcherRegexFilter = []string{"Buffers", "Cached", "MemFree", "Slab"}
var MemRuleAnnotations = map[string]string{}

func NewMemRule(
	deviceFilters map[string]*alertingv1.MemoryInfo,
	usageTypes []string,
	operation string,
	expectedRatio float64,
	duration *durationpb.Duration,
	annotations map[string]string,
) (*AlertingRule, error) {

	dur := model.Duration(duration.AsDuration())

	filters := NewPrometheusFilters()
	filters.AddFilter(NodeFilter)
	for node, state := range deviceFilters {
		filters.Or(NodeFilter, node)
		for _, device := range state.Devices {
			filters.Or(MemoryDeviceFilter, device)
		}
	}
	filters.Match(NodeFilter)
	outputFilters := filters.Build()
	aggrMetrics := ""
	for _, utype := range usageTypes {
		if !slices.Contains(MemoryMatcherRegexFilter, utype) {
			continue // FIXME: warn
		}
		if aggrMetrics != "" {
			aggrMetrics += " + "
		}
		aggrMetrics += fmt.Sprintf("node_memory_%s_bytes%s", utype, outputFilters)
	}
	tmpl := template.Must(template.New("").Parse(`
	(1 -  (
		  {{ .AggrMetrics }}
		)
	/
	  node_memory_MemTotal_bytes{{ .Filters }}
	) {{ .Operation }} bool {{ .ExpectedValue }}
`))
	var b bytes.Buffer
	err := tmpl.Execute(&b, map[string]string{
		"Filters":       filters.Build(),
		"AggrMetrics":   aggrMetrics,
		"Operation":     operation,
		"ExpectedValue": fmt.Sprintf("%.7f", expectedRatio),
	})
	if err != nil {
		return nil, err
	}
	return &AlertingRule{
		Alert:       "",
		Expr:        b.String(),
		For:         dur,
		Labels:      annotations,
		Annotations: annotations,
	}, nil
}
