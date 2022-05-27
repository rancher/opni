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
	return `helm install opni-monitoring-agent ` +
		`{{ arg "input" "Namespace" "+omitEmpty" "+default:opni-monitoring-agent" "+format:-n {{ value }}" }} ` +
		`oci://ghcr.io/kralicky/helm/opni-monitoring-agent --version=0.4.1 ` +
		`--set "token={{ .Token }},pin={{ .Pin }},address={{ .Address }}" ` +
		`{{ arg "toggle" "Install Prometheus Operator" "+omitEmpty" "+default:false" "+format:--set kube-prometheus-stack.enabled={{ value }}" }} ` +
		`--create-namespace`
}
