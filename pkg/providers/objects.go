package providers

import (
	loggingv1beta1 "github.com/banzaicloud/logging-operator/pkg/sdk/api/v1beta1"
	"github.com/rancher/opni/apis/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func BuildLogging(adapter *v1beta1.LogAdapter) *loggingv1beta1.Logging {
	return &loggingv1beta1.Logging{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: adapter.GetNamespace(),
		},
		Spec: loggingv1beta1.LoggingSpec{
			ControlNamespace: adapter.GetNamespace(),
			FluentbitSpec:    adapter.Spec.Fluentbit,
			FluentdSpec:      adapter.Spec.Fluentd,
		},
	}
}

var fluentBitConf = `
[SERVICE]
	Flush             1
	Grace             5
	Daemon            Off
	Log_Level         info
	Coro_Stack_Size   24576
[INPUT]
	Name              systemd
	Tag               k3s
	Path              {{.K3S.LogPath}}
	Systemd_Filter    _SYSTEMD_UNIT=k3s.service
	Strip_Underscores On
[FILTER]
	Name              lua
	Match             *
	script            received_at.lua
	call              append
[OUTPUT]
	Name              forward
	Match             *
	Host              {{ .Release.Name }}-fluentd.{{ .Release.Namespace }}.svc
	Port              24240
	Retry_Limit       False
`

var receivedAtLua = `
function append(tag, timestamp, record)
	new_record = record
	new_record["timestamp"] = os.date("!%Y-%m-%dT%TZ",t)
	return 1, timestamp, new_record
end
`

func BuildK3SConfig(adapter *v1beta1.LogAdapter) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: adapter.GetNamespace(),
		},
		Data: map[string]string{
			"fluent-bit.conf": "",
			"received_at.lua": "",
		},
	}
}
