package metrics

import (
	"bytes"
	"fmt"
	"strconv"
	"text/template"

	"github.com/prometheus/common/model"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

const CPUMatcherName = "node_cpu_seconds_total"
const ModeFilter = "mode"

var CpuRuleAnnotations = map[string]string{}

const CpuFilter = "cpu"

func NewCpuRule(
	nodeFilters map[string]*alertingv1.Cores,
	cpuStates []string,
	operation string,
	expectedRatio float64,
	duration *durationpb.Duration,
	annotations map[string]string,
) (*AlertingRule, error) {

	dur := model.Duration(duration.AsDuration())
	filters := NewPrometheusFilters()
	for node, state := range nodeFilters {
		filters.Or(NodeFilter, node)
		for _, cpu := range state.Items {
			filters.Or(CpuFilter, strconv.Itoa(int(cpu)))
		}
	}
	for _, state := range cpuStates {
		filters.Or(ModeFilter, state)
	}
	filters.Match(ModeFilter)
	filters.Match(NodeFilter)
	filters.Match(CpuFilter)
	// group operations mess this up
	tmpl := template.Must(template.New("").Parse(`
	sum(
		rate(
			node_cpu_seconds_total{{ .Filters }}[1m]
		)
	)
	/
	sum(
		rate(
			node_cpu_seconds_total[1m]
		)
	) {{ .Operation }} bool {{ .ExpectedValue }}
`))
	var b bytes.Buffer
	err := tmpl.Execute(&b, map[string]string{
		"Filters":       filters.Build(),
		"Operation":     operation,
		"ExpectedValue": fmt.Sprintf("%.7f", expectedRatio),
	})
	if err != nil {
		return nil, err
	}
	return &AlertingRule{
		Alert:       "",
		Expr:        PostProcessRuleString(b.String()),
		For:         dur,
		Labels:      annotations,
		Annotations: annotations,
	}, nil
}

func NewCpuSpikeRule(
	nodeFilters map[string]*alertingv1.Cores,
	cpuStates []string,
	operation string,
	expectedRatio float64,
	numSpikes int64,
	duration *durationpb.Duration,
	spikeWindow *durationpb.Duration,
	annotations map[string]string,
) (*AlertingRule, error) {

	dur := model.Duration(duration.AsDuration())
	spikeDur := model.Duration(spikeWindow.AsDuration())
	filters := NewPrometheusFilters()
	for node, state := range nodeFilters {
		filters.Or(NodeFilter, node)
		for _, cpu := range state.Items {
			filters.Or(CpuFilter, strconv.Itoa(int(cpu)))
		}
	}
	for _, state := range cpuStates {
		filters.Or(ModeFilter, state)
	}
	filters.Match(ModeFilter)
	filters.Match(NodeFilter)
	filters.Match(CpuFilter)
	// group operations mess this up
	tmpl := template.Must(template.New("").Parse(`
	count_over_time(
		(
			(
				sum(
					rate(
						node_cpu_seconds_total{{.Filters}}[1m]
					)
				)
				/
				sum(
					rate(
						node_cpu_seconds_total[1m]
					)
				)
			) {{ .Operation }} {{ .ExpectedValue }}
		)
		[{{ .SpikeWindow }}:5s] 
	) > bool {{ .NumSpikes}}
`))
	var b bytes.Buffer
	err := tmpl.Execute(&b, map[string]string{
		"Filters":       filters.Build(),
		"Operation":     operation,
		"ExpectedValue": fmt.Sprintf("%.7f", expectedRatio),
		"SpikeWindow":   spikeDur.String(),
		"NumSpikes":     fmt.Sprintf("%d", numSpikes),
	})
	if err != nil {
		return nil, err
	}
	return &AlertingRule{
		Alert:       "",
		Expr:        PostProcessRuleString(b.String()),
		For:         dur,
		Labels:      annotations,
		Annotations: annotations,
	}, nil
}
