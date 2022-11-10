package metrics

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/prometheus/common/model"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
	"google.golang.org/protobuf/types/known/durationpb"
)

const FilesystemMatcherName = "node_filesystem_free_bytes"
const NodeExporterMountpointLabel = "mountpoint"
const NodeExportFilesystemDeviceLabel = "device"

var FilesystemRuleAnnotations = map[string]string{}

func NewFsRule(
	nodeFilters map[string]*alertingv1.FilesystemInfo,
	operation string,
	expectedValue float64,
	duration *durationpb.Duration,
	annotations map[string]string,
) (*AlertingRule, error) {
	dur := model.Duration(duration.AsDuration())
	filters := NewPrometheusFilters()
	filters.AddFilter(NodeFilter)
	filters.AddFilter(NodeExporterMountpointLabel)
	filters.AddFilter(NodeExportFilesystemDeviceLabel)
	for node, info := range nodeFilters {
		filters.Or(NodeFilter, node)
		for _, device := range info.Devices {
			filters.Or(NodeExportFilesystemDeviceLabel, device)
		}
		for _, mountpoint := range info.Mountpoints {
			filters.Or(NodeFilter, mountpoint)
		}
	}
	filters.Match(NodeFilter)
	filters.Match(NodeExporterMountpointLabel)
	filters.Match(NodeExportFilesystemDeviceLabel)
	tmpl := template.Must(template.New("").Parse(`
		sum(
			1- (
				node_filesystem_free_bytes{{ .Filters }}
				) 
				/ 
				node_filesystem_size_bytes
			)  {{  .Operation }} bool {{ .ExpectedValue }} 
	`))

	var b bytes.Buffer
	err := tmpl.Execute(&b, map[string]string{
		"Filters":       filters.Build(),
		"Operation":     operation,
		"ExpectedValue": fmt.Sprintf("%.7f", expectedValue),
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
