package cortex

import corev1 "github.com/rancher/opni/pkg/apis/core/v1"

func (p *Plugin) CanInstall() error {
	return nil
}

func (p *Plugin) Install(cluster *corev1.Reference) error {
	return nil
}

func (p *Plugin) Uninstall(clustre *corev1.Reference) error {
	return nil
}

func (p *Plugin) InstallerTemplate() string {
	return `helm install opni-agent ` +
		`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-agent" "+format:-n {{ value }}" }} ` +
		`oci://docker.io/rancher/opni-helm --version=0.5.4 ` +
		`--set monitoring.enabled=true,token={{ .Token }},pin={{ .Pin }},address={{ arg "input" "Gateway Hostname" "+default:{{ .Address }}" }}:{{ arg "input" "Gateway Port" "+default:{{ .Port }}" }} ` +
		`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
		`--create-namespace`
}
