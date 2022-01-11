//go:build tools
// +build tools

package main

import (
	_ "github.com/golang/mock/mockgen"
	_ "github.com/onsi/ginkgo/v2/ginkgo"
)
