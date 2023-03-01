package templates

import (
	"fmt"

	"github.com/rancher/opni/pkg/alerting/shared"
	alertingv1 "github.com/rancher/opni/pkg/apis/alerting/v1"
)

func iterateAlerts(innerTemplate string) string {
	return fmt.Sprintf("{{ $renderItem := len .Alerts | ne 1 }}{{ range $i, $val := .Alerts }}%s{{ end }}", innerTemplate)
}

func messageHeader() string {
	return fmt.Sprintf(
		`{{ if $renderItem }}({{$i}}) : {{ end }}{{if .Annotations.%s}}Alarm "{{ .Annotations.%s }}" [{{ .Status }}]{{else}}Notification "{{ .Annotations.%s }}" {{end}}`,
		shared.OpniAlarmNameAnnotation,
		shared.OpniAlarmNameAnnotation,
		shared.OpniHeaderAnnotations,
	)
}

func genericMessageTitle() string {
	return fmt.Sprintf(
		"[{{ .Labels.%s }}] -- {{  .StartsAt | formatTime }}\n",
		alertingv1.NotificationPropertySeverity,
		// time.RFC822,
	)
}

func HeaderTemplate() string {
	return iterateAlerts(messageHeader() + " " + genericMessageTitle())
}

func BodyTemplate() string {
	return iterateAlerts(fmt.Sprintf("{{ if $renderItem }}({{$i}}) : {{ end }}{{ .Annotations.%s }} : {{ .Annotations.%s }}\n", shared.OpniHeaderAnnotations, shared.OpniBodyAnnotations))
}
