//go:build tools

package tools

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "sigs.k8s.io/controller-tools/cmd/controller-gen"
)
