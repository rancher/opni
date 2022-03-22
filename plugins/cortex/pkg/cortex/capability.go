package cortex

import "github.com/rancher/opni-monitoring/pkg/core"

func (p *Plugin) CanInstall() error {
	return nil
}

func (p *Plugin) Install(cluster *core.Reference) error {
	return nil
}

func (p *Plugin) InstallerTemplate() string {
	return "helm install opni-monitoring-agent -n opni-monitoring-agent " +
		"oci://ghcr.io/kralicky/helm/opni-monitoring-agent --version=0.1.0 " +
		`--set token={{ .Token }},pin={{ .Pin }},address={{ .Address }} ` +
		"--create-namespace"
}
