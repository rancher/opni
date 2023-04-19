//go:build tools
// +build tools

package main

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/mikefarah/yq/v4"
	_ "github.com/onsi/ginkgo/v2/ginkgo"
	_ "go.opentelemetry.io/collector/cmd/builder"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
	_ "sigs.k8s.io/kustomize/kustomize/v4"
)
