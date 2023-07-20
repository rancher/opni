package templates

import (
	"fmt"

	"github.com/rancher/opni/pkg/alerting/message"
)

func iterateAlerts(innerTemplate string) string {
	return fmt.Sprintf("{{ $renderItem := len .Alerts | ne 1 }}{{ range $i, $val := .Alerts }}%s{{ end }}", innerTemplate)
}

func clusterNameTmpl() string {
	return fmt.Sprintf(
		`{{ if .Annotations.%s }} cluster "{{ .Annotations.%s }}"{{ else if .Annotations.%s  }} cluster "{{index .Annotations.%s}}"{{ else }} cluster "<not-found>"{{ end }} `,
		message.NotificationContentClusterName,
		message.NotificationContentClusterName,
		message.NotificationPropertyClusterId,
		message.NotificationPropertyClusterId,
	)
}

func messageHeader() string {
	return fmt.Sprintf(
		`{{ if $renderItem }}({{$i}}) : {{ end }}{{if .Annotations.%s}}Alarm "{{ .Annotations.%s }}" for %s [{{ .Status }}]{{else}}Notification "{{ .Annotations.%s }}" {{end}}`,
		message.NotificationContentAlarmName,
		message.NotificationContentAlarmName,
		clusterNameTmpl(),
		message.NotificationContentHeader,
	)
}

func genericMessageTitle() string {
	return fmt.Sprintf(
		"[{{ .Labels.%s }}] -- {{  .StartsAt | formatTime }}\n",
		message.NotificationPropertySeverity,
		// time.RFC822,
	)
}

func HeaderTemplate() string {
	return iterateAlerts(messageHeader() + " " + genericMessageTitle())
}

func BodyTemplate() string {
	return iterateAlerts(fmt.Sprintf("{{ if $renderItem }}({{$i}}) : {{ end }}{{ .Annotations.%s }} : {{ .Annotations.%s }}\n", message.NotificationContentHeader, message.NotificationContentSummary))
}
