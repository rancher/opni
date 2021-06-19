// Package staging provides a manifest of Kubernetes YAML resources needed
// to deploy the Opni Manager. These resources are generated using generate.sh
// which will be invoked when `go generate` is run.
package staging

import _ "embed" // embed should be a blank import

// StagingAutogenYaml contains the full manifest of resources for the Opni
// Manager. This string will contain multiple YAML documents, each containing
// one Kubernetes resource.
//go:embed staging_autogen.yaml
var StagingAutogenYaml string
